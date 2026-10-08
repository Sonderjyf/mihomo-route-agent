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
	baseline, err := readSupervisedLease(ownership)
	if err != nil {
		return err
	}
	if err := checkSupervisedLease(ownership, baseline, time.Now); err != nil {
		return err
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
	return superviseLeasedChild(ctx, child, ownership, baseline, func(pid int) error { return recoverSupervised(c, ownership, state, secret, pid) })
}

func readSupervisedLease(path string) (route.Ownership, error) {
	file, err := os.Open(path)
	if err != nil {
		return route.Ownership{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 16385))
	var lease route.Ownership
	if err != nil || len(body) > 16384 || json.Unmarshal(body, &lease) != nil || lease.Version != 1 || lease.CorePID < 1 || lease.Expires.IsZero() {
		return lease, fmt.Errorf("invalid supervisor lease")
	}
	return lease, nil
}

func checkSupervisedLease(path string, baseline route.Ownership, clock func() time.Time) error {
	current, err := readSupervisedLease(path)
	if err != nil {
		return err
	}
	now := clock() // read after the file: a concurrent renewal may postdate the tick
	expires := current.Expires
	current.Expires = baseline.Expires
	if current != baseline {
		return fmt.Errorf("supervised ownership generation changed")
	}
	limit := 2 * time.Minute
	if expires.Equal(baseline.Expires) {
		limit = 10 * time.Minute
	} // initial acquisition only
	if !now.Before(expires) || expires.Sub(now) > limit {
		return fmt.Errorf("supervised lease expired or invalid")
	}
	return nil
}

func watchLease(ctx context.Context, path string, baseline route.Ownership, ticks <-chan time.Time, clock func() time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-ticks:
			if !ok {
				return fmt.Errorf("lease supervision clock stopped")
			}
			if err := checkSupervisedLease(path, baseline, clock); err != nil {
				return err
			}
		}
	}
}

func superviseLeasedChild(ctx context.Context, child *exec.Cmd, path string, baseline route.Ownership, recover func(int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	watch := make(chan error, 1)
	go func() {
		err := watchLease(ctx, path, baseline, tick.C, time.Now)
		watch <- err
		if err != nil {
			cancel()
		}
	}()
	err := superviseChild(ctx, child, recover)
	cancel()
	leaseErr := <-watch
	if err != nil {
		return err
	}
	if leaseErr != nil {
		return fmt.Errorf("lease supervisor stopped publication: %w", leaseErr)
	}
	return nil
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
