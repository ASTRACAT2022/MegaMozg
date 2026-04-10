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
		input.Mode = "job_simulated"
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

	shouldRunLocalExec := input.Mode == "local_exec" && os.Getenv("MEZA_TERMINAL_LOCAL_EXEC_ENABLED") == "true"
	if shouldRunLocalExec {
		writeSSE(w, "line", map[string]any{
			"line": "[info] local_exec enabled: command runs on meza-core host (not remote node)",
		})
		flusher.Flush()
		if err := streamLocalExec(r.Context(), w, flusher, input.Command); err != nil {
			writeSSE(w, "error", map[string]any{"error": err.Error(), "job_id": job.ID})
			flusher.Flush()
			return
		}
	} else {
		if input.Mode == "local_exec" {
			writeSSE(w, "line", map[string]any{
				"line": "[warn] local_exec disabled by MEZA_TERMINAL_LOCAL_EXEC_ENABLED=false, fallback to simulated output",
			})
			flusher.Flush()
		}
		streamSimulatedOutput(r.Context(), w, flusher, input)
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
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			lines <- streamLine{prefix: prefix, text: scanner.Text()}
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
				return nil
			}
			writeSSE(w, "line", map[string]any{
				"line": fmt.Sprintf("[%s] %s", line.prefix, line.text),
			})
			flusher.Flush()
		}
	}
}

func writeSSE(w io.Writer, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"error":"marshal failed"}`)
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
