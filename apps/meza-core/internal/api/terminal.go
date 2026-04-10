package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/mezamozg/meza-core/internal/domain"
)

type terminalStreamRequest struct {
	NodeName string `json:"node_name"`
	Command  string `json:"command"`
	Mode     string `json:"mode"`
}

func (s *Server) handleTerminalStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	var input terminalStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	input.NodeName = strings.TrimSpace(input.NodeName)
	input.Command = strings.TrimSpace(input.Command)
	input.Mode = strings.TrimSpace(input.Mode)
	if input.Mode == "" {
		input.Mode = "ssh_exec"
	}

	if input.NodeName == "" || input.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_name and command are required"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeSSE(w, "status", map[string]any{
		"status": "starting",
		"node":   input.NodeName,
		"mode":   input.Mode,
	})
	flusher.Flush()

	job := s.store.CreateJob(domain.JobCreateInput{
		Type:           "shell_command",
		TargetSelector: "node:" + input.NodeName,
		Strategy:       "all-at-once",
		Payload: map[string]any{
			"command": input.Command,
		},
		CreatedBy: "manual-terminal",
		Summary:   fmt.Sprintf("Terminal command on %s", input.NodeName),
	}, nil)

	_, _ = s.store.ApproveJob(job.ID, "manual-terminal")
	_, _ = s.store.StartJob(job.ID, "manual-terminal")

	writeSSE(w, "status", map[string]any{
		"status": "job_created",
		"job_id": job.ID,
	})
	flusher.Flush()

	switch input.Mode {
	case "ssh_exec":
		if os.Getenv("MEZA_TERMINAL_SSH_ENABLED") != "true" {
			writeSSE(w, "error", map[string]any{
				"error":  "ssh_exec disabled. Set MEZA_TERMINAL_SSH_ENABLED=true",
				"job_id": job.ID,
			})
			flusher.Flush()
			return
		}

		node, found := findTerminalNode(s.store.ListNodes(), input.NodeName)
		if !found {
			writeSSE(w, "error", map[string]any{
				"error":  fmt.Sprintf("node %q not found", input.NodeName),
				"job_id": job.ID,
			})
			flusher.Flush()
			return
		}

		nodeIP := strings.TrimSpace(node.IPAddress)
		if nodeIP == "" {
			writeSSE(w, "error", map[string]any{
				"error":  fmt.Sprintf("node %q has no ip_address. Reinstall/reconnect node to publish IP.", node.Name),
				"job_id": job.ID,
			})
			flusher.Flush()
			return
		}

		writeSSE(w, "line", map[string]any{
			"line": fmt.Sprintf("[info] ssh_exec: connecting to %s (%s)", node.Name, nodeIP),
		})
		flusher.Flush()
		if err := streamSSHExec(r.Context(), w, flusher, nodeIP, input.Command); err != nil {
			writeSSE(w, "error", map[string]any{"error": err.Error(), "job_id": job.ID})
			flusher.Flush()
			return
		}
	case "local_exec":
		if os.Getenv("MEZA_TERMINAL_LOCAL_EXEC_ENABLED") != "true" {
			writeSSE(w, "error", map[string]any{
				"error":  "local_exec disabled. Set MEZA_TERMINAL_LOCAL_EXEC_ENABLED=true",
				"job_id": job.ID,
			})
			flusher.Flush()
			return
		}

		writeSSE(w, "line", map[string]any{
			"line": "[info] local_exec: command runs on meza-core container",
		})
		flusher.Flush()
		if err := streamLocalExec(r.Context(), w, flusher, input.Command); err != nil {
			writeSSE(w, "error", map[string]any{"error": err.Error(), "job_id": job.ID})
			flusher.Flush()
			return
		}
	case "job_simulated":
		streamSimulatedOutput(r.Context(), w, flusher, input)
	default:
		writeSSE(w, "error", map[string]any{
			"error":  fmt.Sprintf("unknown terminal mode: %s", input.Mode),
			"job_id": job.ID,
		})
		flusher.Flush()
		return
	}

	writeSSE(w, "done", map[string]any{
		"status": "completed",
		"job_id": job.ID,
	})
	flusher.Flush()
}

func streamSimulatedOutput(ctx context.Context, w io.Writer, flusher http.Flusher, input terminalStreamRequest) {
	lines := []string{
		fmt.Sprintf("$ %s", input.Command),
		fmt.Sprintf("[node:%s] connecting...", input.NodeName),
		fmt.Sprintf("[node:%s] running command", input.NodeName),
		"[stdout] command accepted by control-plane",
		"[stdout] execution completed",
	}

	for _, line := range lines {
		select {
		case <-ctx.Done():
			return
		case <-time.After(220 * time.Millisecond):
			writeSSE(w, "line", map[string]any{"line": line})
			flusher.Flush()
		}
	}
}

func streamLocalExec(ctx context.Context, w io.Writer, flusher http.Flusher, command string) error {
	cmd := exec.CommandContext(ctx, "/bin/bash", "-lc", command)
	return streamExecCommand(ctx, w, flusher, cmd, false)
}

func streamSSHExec(ctx context.Context, w io.Writer, flusher http.Flusher, nodeIP, command string) error {
	sshUser := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_USER"))
	if sshUser == "" {
		sshUser = "root"
	}

	sshPort := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_PORT"))
	if sshPort == "" {
		sshPort = "22"
	}

	sshKeyPath := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_KEY_PATH"))
	strictHostKeyChecking := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_STRICT_HOST_KEY_CHECKING"))
	if strictHostKeyChecking == "" {
		strictHostKeyChecking = "accept-new"
	}

	userKnownHostsFile := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_KNOWN_HOSTS_PATH"))
	if userKnownHostsFile == "" {
		userKnownHostsFile = "/data/ssh/known_hosts"
	}

	args := []string{
		"-p", sshPort,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", fmt.Sprintf("StrictHostKeyChecking=%s", strictHostKeyChecking),
		"-o", fmt.Sprintf("UserKnownHostsFile=%s", userKnownHostsFile),
	}
	if sshKeyPath != "" {
		args = append(args, "-i", sshKeyPath)
	}

	target := fmt.Sprintf("%s@%s", sshUser, nodeIP)
	remoteCommand := fmt.Sprintf("export SYSTEMD_PAGER=cat PAGER=cat; %s", command)
	args = append(args, target, remoteCommand)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	return streamExecCommand(ctx, w, flusher, cmd, true)
}

func streamExecCommand(ctx context.Context, w io.Writer, flusher http.Flusher, cmd *exec.Cmd, rawStdout bool) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	type streamLine struct {
		prefix string
		text   string
	}
	lines := make(chan streamLine, 64)
	var wg sync.WaitGroup

	readPipe := func(prefix string, pipe io.Reader) {
		defer wg.Done()
		reader := bufio.NewReaderSize(pipe, 64*1024)
		for {
			chunk, readErr := reader.ReadString('\n')
			if chunk != "" {
				lines <- streamLine{
					prefix: prefix,
					text:   strings.TrimRight(chunk, "\r\n"),
				}
			}

			if readErr == io.EOF {
				return
			}
			if readErr != nil {
				lines <- streamLine{prefix: prefix, text: fmt.Sprintf("[read-error] %v", readErr)}
				return
			}
		}
	}

	wg.Add(2)
	go readPipe("stdout", stdout)
	go readPipe("stderr", stderr)

	waitErr := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		waitErr <- err
		wg.Wait()
		close(lines)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-lines:
			if !ok {
				if err := <-waitErr; err != nil {
					return fmt.Errorf("command failed: %w", err)
				}
				writeSSE(w, "line", map[string]any{
					"line": "[info] command finished",
				})
				flusher.Flush()
				return nil
			}
			text := line.text
			if line.prefix == "stderr" {
				text = fmt.Sprintf("[stderr] %s", line.text)
			}
			if !rawStdout && line.prefix == "stdout" {
				text = fmt.Sprintf("[stdout] %s", line.text)
			}
			writeSSE(w, "line", map[string]any{
				"line": text,
			})
			flusher.Flush()
		}
	}
}

func findTerminalNode(nodes []domain.Node, selector string) (domain.Node, bool) {
	for _, node := range nodes {
		if node.Name == selector || node.DisplayName == selector || node.ID == selector {
			return node, true
		}
	}
	return domain.Node{}, false
}

func writeSSE(w io.Writer, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"error":"marshal failed"}`)
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
