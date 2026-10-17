package route

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// Diagnostics deliberately exclude command text, paths and raw output/stderr.
type windowsQueryError struct {
	Operation     string
	Stage         string
	ExitCode      int
	StderrPresent bool
}

func (e *windowsQueryError) Error() string {
	return fmt.Sprintf("windows_query_%s: %s (exit_code=%d, stderr_present=%t)", e.Operation, e.Stage, e.ExitCode, e.StderrPresent)
}

type queryOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (b *queryOutput) Len() int      { return b.buffer.Len() }
func (b *queryOutput) Bytes() []byte { return b.buffer.Bytes() }
func (b *queryOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 16384 - b.Len()
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

func queryFailure(operation, stage string, exit int, stderr bool) error {
	return &windowsQueryError{operation, stage, exit, stderr}
}

func runWindowsQuery(ctx context.Context, operation, script string, budget time.Duration) ([]byte, error) {
	return runWindowsQueryWith(ctx, operation, script, budget, func(ctx context.Context, script string) *exec.Cmd {
		return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	})
}

func runWindowsQueryWith(ctx context.Context, operation, script string, budget time.Duration, command func(context.Context, string) *exec.Cmd) ([]byte, error) {
	check, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	cmd := command(check, script)
	cmd.WaitDelay = time.Second
	var stdout, stderr queryOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if check.Err() != nil {
		stage := "canceled"
		if errors.Is(check.Err(), context.DeadlineExceeded) {
			stage = "timeout"
		}
		return nil, queryFailure(operation, stage, -1, stderr.Len() > 0)
	}
	if err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return nil, queryFailure(operation, "exit_failed", exited.ExitCode(), stderr.Len() > 0)
		}
		stage := "start_failed"
		if cmd.ProcessState != nil {
			stage = "io_failed"
		}
		return nil, queryFailure(operation, stage, -1, stderr.Len() > 0)
	}
	if stdout.overflow || stderr.overflow {
		return nil, queryFailure(operation, "output_limit", 0, stderr.Len() > 0)
	}
	return bytes.TrimSpace(stdout.Bytes()), nil
}
