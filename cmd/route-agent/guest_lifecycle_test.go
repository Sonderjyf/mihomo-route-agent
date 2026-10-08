package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/Sonderjyf/mihomo-route-agent/internal/route"
)

func TestGuestLifecycleSupervisor(t *testing.T) {
	if os.Getenv("ROUTE_AGENT_GUEST_ACCEPTANCE") != "1" {
		t.Skip("explicit guest acceptance only")
	}
	if runtime.GOOS != "windows" || os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" || os.Getenv("GITHUB_REPOSITORY") != "Sonderjyf/mihomo-route-agent" || os.Getenv("OPENROUTER_API_KEY") != "" {
		t.Fatal("not an authorized synthetic guest")
	}
	c, err := route.LoadConfig(os.Getenv("ROUTE_AGENT_GUEST_CONFIG"), true)
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Getenv("ROUTE_AGENT_GUEST_WORKER"), "-test.run=^TestGuestControlledLifecycleWorker$", "-test.timeout=10m")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	ctx, stop := context.WithTimeout(context.Background(), 9*time.Minute)
	defer stop()
	if err := superviseChild(ctx, child, func(pid int) error {
		return recoverSupervised(c, os.Getenv("ROUTE_AGENT_GUEST_OWNERSHIP"), os.Getenv("ROUTE_AGENT_GUEST_STATE"), os.Getenv("MIHOMO_SECRET"), pid)
	}); err != nil {
		t.Fatal(err)
	}
}
