package main

import (
	"context"
	"encoding/json"
	"github.com/Sonderjyf/mihomo-route-agent/internal/route"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHeartbeatEOFInvalidAndTimeoutStopPublisher(t *testing.T) {
	for _, kind := range []string{"eof", "invalid", "timeout"} {
		t.Run(kind, func(t *testing.T) {
			r, w := io.Pipe()
			defer r.Close()
			defer w.Close()
			ctx, stop := heartbeatContext(context.Background(), r, 20*time.Millisecond)
			defer stop()
			if kind == "eof" {
				w.Close()
			} else if kind == "invalid" {
				io.WriteString(w, "INVALID\n")
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("publisher kept running")
			}
		})
	}
}

// Test binary child: never invokes route-agent networking or the real app.
func TestSupervisionProcessHelper(t *testing.T) {
	mode := os.Getenv("ROUTE_AGENT_PROCESS_TEST")
	if mode == "" {
		return
	}
	if mode == "crash" {
		os.Exit(7)
	}
	ctx, stop := heartbeatContext(context.Background(), os.Stdin, time.Second)
	defer stop()
	<-ctx.Done()
	os.Exit(0)
}

func TestIndependentSupervisorWaitsThenRecoversWithoutRestart(t *testing.T) {
	for _, mode := range []string{"crash", "graceful"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.Command(os.Args[0], "-test.run=^TestSupervisionProcessHelper$")
			cmd.Env = append(os.Environ(), "ROUTE_AGENT_PROCESS_TEST="+mode)
			calls := 0
			if mode == "graceful" {
				time.AfterFunc(20*time.Millisecond, cancel)
			}
			err := superviseChild(ctx, cmd, func(pid int) error {
				calls++
				if pid != cmd.Process.Pid || cmd.ProcessState == nil {
					t.Fatal("recovery before confirmed exit")
				}
				return nil
			})
			if calls != 1 {
				t.Fatal("recovery missing/repeated")
			}
			if mode == "crash" && (err == nil || !strings.Contains(err.Error(), "learning remains stopped")) {
				t.Fatal(err)
			}
			if mode == "graceful" && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCanceledSupervisorDoesNotLaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := exec.Command("this-executable-must-not-be-started")
	if err := superviseChild(ctx, cmd, func(int) error { t.Fatal("unexpected recovery"); return nil }); err != context.Canceled {
		t.Fatal(err)
	}
	if cmd.Process != nil {
		t.Fatal("process launched after cancellation")
	}
}

func TestIndependentLeaseWatchUsesControlledClock(t *testing.T) {
	now := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	baseline := route.Ownership{Version: 1, CorePID: 123, Expires: now.Add(10 * time.Minute), Token: "synthetic"}
	path := filepath.Join(t.TempDir(), "lease.json")
	write := func(lease route.Ownership) {
		body, _ := json.Marshal(lease)
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(baseline)
	if err := checkSupervisedLease(path, baseline, clock); err != nil {
		t.Fatal(err)
	}
	renewed := baseline
	renewed.Expires = now.Add(2 * time.Minute)
	write(renewed)
	if err := checkSupervisedLease(path, baseline, clock); err != nil {
		t.Fatal(err)
	}
	ticks := make(chan time.Time, 1)
	ticks <- now.Add(2 * time.Minute)
	now = now.Add(2 * time.Minute)
	if err := watchLease(context.Background(), path, baseline, ticks, clock); err == nil {
		t.Fatal("expired unresponsive worker not detected")
	}
	renewed.Token = "different"
	write(renewed)
	if err := checkSupervisedLease(path, baseline, clock); err == nil {
		t.Fatal("changed owner accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := checkSupervisedLease(path, baseline, clock); err == nil {
		t.Fatal("missing lease accepted")
	}
}
