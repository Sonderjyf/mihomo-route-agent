package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/Sonderjyf/mihomo-route-agent/internal/route"
)

// A closed pipe, invalid frame or missing heartbeat cancels publication even if
// the supervisor is killed. Only the child worker touches publication state.
func heartbeatContext(parent context.Context, input io.Reader, timeout time.Duration) (context.Context, context.CancelFunc) {
	return route.SupervisorContext(parent, input, timeout)
}

// superviseChild owns exactly the launched PID. Confirmed Wait precedes any
// recovery. Cancellation requests graceful drain through EOF, then bounds exit.
func superviseChild(ctx context.Context, child *exec.Cmd, recover func(int) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	input, err := child.StdinPipe()
	if err != nil {
		return err
	}
	defer input.Close()
	if err := child.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	beats, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			if _, err := io.WriteString(input, "PING\n"); err != nil {
				return
			}
			select {
			case <-beats.Done():
				return
			case <-tick.C:
			}
		}
	}()
	var exitErr error
	select {
	case exitErr = <-done:
	case <-ctx.Done():
		stop()
		_ = input.Close()
		timer := time.NewTimer(20 * time.Second)
		defer timer.Stop()
		select {
		case exitErr = <-done:
		case <-timer.C:
			_ = child.Process.Kill()
			exitErr = <-done
		}
	}
	stop()
	_ = input.Close()
	if err := recover(child.Process.Pid); err != nil {
		return fmt.Errorf("publisher stopped; owned recovery incomplete: %w", err)
	}
	if exitErr != nil {
		return fmt.Errorf("publisher exited; owned cleanup completed, learning remains stopped: %w", exitErr)
	}
	return nil
}

func runContinuous(ctx context.Context, c route.Config, arguments []string, ownership, state string) error {
	if c.Judge == "jev" && os.Getenv("OPENROUTER_API_KEY") == "" {
		return fmt.Errorf("missing explicitly authorized model credential")
	}
	secret := os.Getenv("MIHOMO_SECRET")
	if secret == "" {
		return fmt.Errorf("continuous supervision requires controller authentication")
	}
	lock, err := os.OpenFile(ownership+".supervisor.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("supervisor ownership already held or interrupted")
	}
	defer os.Remove(ownership + ".supervisor.lock")
	defer lock.Close()
	if err := json.NewEncoder(lock).Encode(map[string]int{"supervisor_pid": os.Getpid()}); err != nil {
		return err
	}
	if err := lock.Sync(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	child := exec.Command(exe, append([]string{"observe-apply"}, append(arguments, "--supervised-worker")...)...)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	return superviseChild(ctx, child, func(pid int) error { return recoverSupervised(c, ownership, state, secret, pid) })
}

func recoverSupervised(c route.Config, ownership, state, secret string, pid int) error {
	body, err := os.ReadFile(ownership + ".lock")
	if os.IsNotExist(err) {
		return nil
	} // child removes it only after verified cleanup
	var record struct {
		Version int `json:"version"`
		PID     int `json:"pid"`
	}
	if err != nil || json.Unmarshal(body, &record) != nil || record.Version != 1 || record.PID != pid {
		return fmt.Errorf("publisher lock does not identify supervised child")
	}
	recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return route.RecoverControlled(recovery, c, true, ownership, state, secret)

}
