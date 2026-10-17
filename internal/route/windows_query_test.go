package route

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWindowsQueryErrorLayersAndRedaction(t *testing.T) {
	for _, test := range []struct {
		kind, stage string
		code        int
	}{
		{"start", "start_failed", -1}, {"exit", "exit_failed", 23}, {"timeout", "timeout", -1}, {"cancel", "canceled", -1}, {"large", "output_limit", 0},
	} {
		t.Run(test.kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			budget := 5 * time.Second
			if test.kind == "timeout" {
				budget = 20 * time.Millisecond
			}
			if test.kind == "cancel" {
				cancel()
			}
			_, err := runWindowsQueryWith(ctx, "test", "private-command-token", budget, func(ctx context.Context, _ string) *exec.Cmd {
				if test.kind == "start" {
					return exec.CommandContext(ctx, "nonexistent-private-executable-path")
				}
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWindowsQueryChild$")
				cmd.Env = append(os.Environ(), "ROUTE_AGENT_QUERY_TEST="+test.kind)
				return cmd
			})
			var query *windowsQueryError
			if !errors.As(err, &query) || query.Stage != test.stage || query.ExitCode != test.code {
				t.Fatalf("wrong safe error: %v", err)
			}
			if strings.Contains(err.Error(), "private-") {
				t.Fatal("private command/output leaked")
			}
		})
	}
}

func TestWindowsQueryChild(t *testing.T) {
	switch os.Getenv("ROUTE_AGENT_QUERY_TEST") {
	case "exit":
		fmt.Fprintln(os.Stderr, "private-user-path private-command-token")
		os.Exit(23)
	case "large":
		fmt.Print(strings.Repeat("x", 17000))
		os.Exit(0)
	case "timeout":
		for range time.NewTicker(time.Second).C {
		}
	}
}
