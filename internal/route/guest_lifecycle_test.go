package route

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Test-binary-only adapter. No synthetic path/decision switch is compiled into
// route-agent.exe. This is NOT physical-path or real model acceptance.
type guestSyntheticPath struct{}

func (guestSyntheticPath) Check(context.Context, netip.Addr) error { return nil }
func (guestSyntheticPath) CheckOwner(ctx context.Context, lease Ownership) error {
	return (windowsDirectPath{}).CheckOwner(ctx, lease)
}

type guestSyntheticCollector struct{}

func (guestSyntheticCollector) Collect(_ context.Context, d Domain) (ProbeReport, error) {
	if !strings.HasSuffix(d.Host, ".route-lab.test") {
		return ProbeReport{}, fmt.Errorf("synthetic allowlist only")
	}
	return ProbeReport{Evidence: Evidence{"repeated_failure", "verified_success"}}, nil
}

func TestGuestControlledLifecycleWorker(t *testing.T) {
	if os.Getenv("ROUTE_AGENT_GUEST_ACCEPTANCE") != "1" {
		t.Skip("explicit guest acceptance only")
	}
	if runtime.GOOS != "windows" || os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" || os.Getenv("GITHUB_REPOSITORY") != "Sonderjyf/mihomo-route-agent" || os.Getenv("OPENROUTER_API_KEY") != "" {
		t.Fatal("not an authorized synthetic guest")
	}
	c, err := LoadConfig(os.Getenv("ROUTE_AGENT_GUEST_CONFIG"), true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Judge != "stub" || c.Observation == nil {
		t.Fatal("synthetic config required")
	}
	for _, h := range c.Observation.Hosts {
		if !strings.HasSuffix(h, ".route-lab.test") {
			t.Fatal("synthetic hosts required")
		}
	}
	o, err := newControlledObserver(c, true, os.Getenv("ROUTE_AGENT_GUEST_OWNERSHIP"), guestSyntheticPath{}, &recordingJudge{}, os.Getenv("MIHOMO_SECRET"))
	if err != nil {
		t.Fatal(err)
	}
	o.collector = guestSyntheticCollector{}
	if err := o.EnableContinuous(true); err != nil {
		t.Fatal(err)
	}
	ctx, stop := SupervisorContext(context.Background(), os.Stdin, 5*time.Second)
	defer stop()
	if err := o.Run(ctx, os.Getenv("ROUTE_AGENT_GUEST_STATE")); err != nil {
		t.Fatal(err)
	}
}
