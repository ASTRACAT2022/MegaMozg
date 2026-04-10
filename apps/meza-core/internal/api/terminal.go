package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

type terminalAgentPollRequest struct {
	NodeName string `json:"node_name"`
}

type terminalAgentTask struct {
	ID        string
	NodeName  string
	Command   string
	Status    string
	Output    string
	Error     string
	ExitCode  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type terminalAgentHub struct {
	mu     sync.Mutex
	seq    int64
	byID   map[string]*terminalAgentTask
	queues map[string][]string
}

var agentExecHub = &terminalAgentHub{
	byID:   map[string]*terminalAgentTask{},
	queues: map[string][]string{},
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
		input.Mode = "agent_exec"
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
	case "agent_exec":
		if strings.EqualFold(strings.TrimSpace(os.Getenv("MEZA_TERMINAL_AGENT_EXEC_ENABLED")), "false") {
			writeSSE(w, "error", map[string]any{
				"error":  "agent_exec disabled by env. Set MEZA_TERMINAL_AGENT_EXEC_ENABLED=true",
				"job_id": job.ID,
			})
			flusher.Flush()
			return
		}

		if err := s.streamAgentExec(r.Context(), w, flusher, input.NodeName, input.Command); err != nil {
			writeSSE(w, "error", map[string]any{"error": err.Error(), "job_id": job.ID})
			flusher.Flush()
			return
		}
	case "ssh_exec":
		if strings.EqualFold(strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_ENABLED")), "false") {
			writeSSE(w, "error", map[string]any{
				"error":  "ssh_exec disabled by env. Set MEZA_TERMINAL_SSH_ENABLED=true",
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

func (s *Server) handleTerminalAgentPoll(w http.ResponseWriter, r *http.Request) {
	var input terminalAgentPollRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}

	nodeName := strings.TrimSpace(input.NodeName)
	if nodeName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node_name is required"})
		return
	}

	task, ok := agentExecHub.claim(nodeName)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("X-Meza-Task-ID", task.ID)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, task.Command)
}

func (s *Server) handleTerminalAgentResult(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimSpace(r.PathValue("id"))
	if taskID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "completed"
	}

	exitCode := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("exit_code")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			exitCode = parsed
		}
	}

	nodeName := strings.TrimSpace(r.URL.Query().Get("node_name"))
	errText := strings.TrimSpace(r.URL.Query().Get("error"))

	rawOutput, _ := io.ReadAll(io.LimitReader(r.Body, 2*1024*1024))
	output := string(rawOutput)

	if err := agentExecHub.complete(taskID, nodeName, status, exitCode, output, errText); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) streamAgentExec(ctx context.Context, w io.Writer, flusher http.Flusher, nodeName, command string) error {
	node, found := findTerminalNode(s.store.ListNodes(), nodeName)
	if !found {
		return fmt.Errorf("node %q not found", nodeName)
	}

	task := agentExecHub.enqueue(node.Name, command)
	writeSSE(w, "line", map[string]any{
		"line": fmt.Sprintf("[info] agent_exec: queued task %s for %s", task.ID, node.Name),
	})
	flusher.Flush()

	timeoutSeconds := 180
	if raw := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_AGENT_TIMEOUT_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeoutSeconds = parsed
		}
	}

	timeout := time.NewTimer(time.Duration(timeoutSeconds) * time.Second)
	defer timeout.Stop()
	poll := time.NewTicker(1 * time.Second)
	defer poll.Stop()

	lastStatus := ""
	waitTicks := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			_ = agentExecHub.complete(task.ID, task.NodeName, "failed", 124, "", "agent_exec timeout")
			return fmt.Errorf("agent_exec timeout after %ds: node %s did not return result", timeoutSeconds, task.NodeName)
		case <-poll.C:
			current, ok := agentExecHub.get(task.ID)
			if !ok {
				return errors.New("agent_exec task disappeared")
			}

			if current.Status != lastStatus {
				lastStatus = current.Status
				writeSSE(w, "line", map[string]any{
					"line": fmt.Sprintf("[info] agent_exec: status=%s", current.Status),
				})
				flusher.Flush()
			}

			if current.Status == "pending" {
				waitTicks++
				if waitTicks%5 == 0 {
					writeSSE(w, "line", map[string]any{
						"line": fmt.Sprintf("[info] waiting node %s to pick task...", current.NodeName),
					})
					flusher.Flush()
				}
				continue
			}

			if current.Status == "running" {
				continue
			}

			output := strings.TrimSpace(current.Output)
			if output != "" {
				for _, line := range strings.Split(output, "\n") {
					writeSSE(w, "line", map[string]any{"line": line})
					flusher.Flush()
				}
			}

			if current.Status == "failed" || current.ExitCode != 0 {
				errText := strings.TrimSpace(current.Error)
				if errText == "" {
					errText = "command failed on node"
				}
				return fmt.Errorf("%s (exit=%d)", errText, current.ExitCode)
			}

			if output == "" {
				writeSSE(w, "line", map[string]any{"line": "[info] command finished (empty output)"})
				flusher.Flush()
			}
			return nil
		}
	}
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
	if sshKeyPath == "" {
		sshKeyPath = "/data/ssh/id_ed25519"
	}
	if _, err := os.Stat(sshKeyPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("ssh key not found: %s. add private key and set MEZA_TERMINAL_SSH_KEY_PATH", sshKeyPath)
		}
		return fmt.Errorf("ssh key access error (%s): %w", sshKeyPath, err)
	}
	strictHostKeyChecking := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_STRICT_HOST_KEY_CHECKING"))
	if strictHostKeyChecking == "" {
		strictHostKeyChecking = "accept-new"
	}

	userKnownHostsFile := strings.TrimSpace(os.Getenv("MEZA_TERMINAL_SSH_KNOWN_HOSTS_PATH"))
	if userKnownHostsFile == "" {
		userKnownHostsFile = "/data/ssh/known_hosts"
	}
	if err := os.MkdirAll(filepath.Dir(userKnownHostsFile), 0o700); err != nil {
		return fmt.Errorf("create known_hosts dir failed: %w", err)
	}

	args := []string{
		"-p", sshPort,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", fmt.Sprintf("StrictHostKeyChecking=%s", strictHostKeyChecking),
		"-o", fmt.Sprintf("UserKnownHostsFile=%s", userKnownHostsFile),
	}
	args = append(args, "-i", sshKeyPath)

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
	stderrTail := make([]string, 0, 8)

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
					return formatExecFailure(err, stderrTail)
				}
				writeSSE(w, "line", map[string]any{
					"line": "[info] command finished",
				})
				flusher.Flush()
				return nil
			}
			text := line.text
			if line.prefix == "stderr" {
				stderrTail = appendStderrTail(stderrTail, line.text)
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

func appendStderrTail(tail []string, line string) []string {
	const maxTail = 6
	line = strings.TrimSpace(line)
	if line == "" {
		return tail
	}
	tail = append(tail, line)
	if len(tail) > maxTail {
		tail = tail[len(tail)-maxTail:]
	}
	return tail
}

func formatExecFailure(err error, stderrTail []string) error {
	if err == nil {
		return nil
	}

	base := fmt.Sprintf("command failed: %v", err)
	if len(stderrTail) == 0 {
		return errors.New(base)
	}

	hint := ""
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 255 {
		hint = "SSH failure: проверь user/ip/port, ключ и доступ ноды."
	}

	joined := strings.Join(stderrTail, " | ")
	if hint != "" {
		return fmt.Errorf("%s | %s | stderr: %s", base, hint, joined)
	}
	return fmt.Errorf("%s | stderr: %s", base, joined)
}

func (h *terminalAgentHub) enqueue(nodeName, command string) terminalAgentTask {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.seq++
	now := time.Now().UTC()
	id := fmt.Sprintf("agent-task-%d", h.seq)
	task := &terminalAgentTask{
		ID:        id,
		NodeName:  nodeName,
		Command:   command,
		Status:    "pending",
		ExitCode:  0,
		CreatedAt: now,
		UpdatedAt: now,
	}

	h.byID[id] = task
	h.queues[nodeName] = append(h.queues[nodeName], id)
	return *task
}

func (h *terminalAgentHub) claim(nodeName string) (terminalAgentTask, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	queue := h.queues[nodeName]
	if len(queue) == 0 {
		return terminalAgentTask{}, false
	}

	id := queue[0]
	h.queues[nodeName] = queue[1:]
	task, ok := h.byID[id]
	if !ok {
		return terminalAgentTask{}, false
	}

	task.Status = "running"
	task.UpdatedAt = time.Now().UTC()
	return *task, true
}

func (h *terminalAgentHub) complete(taskID, nodeName, status string, exitCode int, output, errText string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	task, ok := h.byID[taskID]
	if !ok {
		return errors.New("terminal task not found")
	}
	if nodeName != "" && task.NodeName != nodeName {
		return fmt.Errorf("task belongs to node %s, got %s", task.NodeName, nodeName)
	}

	task.Status = status
	task.ExitCode = exitCode
	task.Output = output
	task.Error = errText
	task.UpdatedAt = time.Now().UTC()
	return nil
}

func (h *terminalAgentHub) get(taskID string) (terminalAgentTask, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	task, ok := h.byID[taskID]
	if !ok {
		return terminalAgentTask{}, false
	}
	return *task, true
}

func (h *terminalAgentHub) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq = 0
	h.byID = map[string]*terminalAgentTask{}
	h.queues = map[string][]string{}
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
