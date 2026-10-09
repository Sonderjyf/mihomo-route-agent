package route

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Opt-in ordinary Windows CI; only our own loopback listener and child process.
func TestWindowsOwnedQueryMechanism(t *testing.T) {
	if os.Getenv("ROUTE_AGENT_QUERY_DIAGNOSTIC") != "1" {
		t.Skip("explicit owned-process diagnostic")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("GITHUB_REPOSITORY") != "Sonderjyf/mihomo-route-agent" {
		t.Fatal("hosted repository diagnostic required")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	controller := fmt.Sprintf("http://127.0.0.1:%d", port)
	started := time.Now()
	identity, err := readCoreIdentity(context.Background(), controller)
	if err != nil || identity.PID != os.Getpid() {
		t.Fatalf("cold production identity rejected own listener: %v", err)
	}
	t.Logf("native_first_query elapsed_ms=%d budget_ms=%d pid_matches_self=true", time.Since(started).Milliseconds(), nativeIdentityBudget.Milliseconds())
	for i := 0; i < 10; i++ {
		again, err := readCoreIdentity(context.Background(), controller)
		if err != nil || again != identity {
			t.Fatal("repeat identity changed", err)
		}
	}
	lease := Ownership{Controller: controller, CorePID: identity.PID, CoreStarted: identity.Started}
	if err := (windowsDirectPath{}).CheckOwner(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	for _, changed := range []Ownership{{Controller: controller, CorePID: identity.PID + 1, CoreStarted: identity.Started}, {Controller: controller, CorePID: identity.PID, CoreStarted: "1"}} {
		if err := (windowsDirectPath{}).CheckOwner(context.Background(), changed); err == nil {
			t.Fatal("changed PID/start identity accepted")
		}
	}
	if exited, err := windowsPublisherExited(context.Background(), os.Getpid()); err != nil || exited {
		t.Fatal("live publisher not recognized", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readCoreIdentity(ctx, controller); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not preserved", err)
	}
	// Get-Process only (no CIM): verify legacy lease epoch after the native cold
	// query. This compatibility cross-check is never a production dependency.
	// Allow cold PowerShell startup its own bounded test budget; native owner
	// queries and the production physical-route guard retain their 3s budgets.
	const compatibilityBudget = 15 * time.Second
	compatibilityStarted := time.Now()
	body, err := runWindowsQuery(context.Background(), "legacy_tick_compatibility", fmt.Sprintf("(Get-Process -Id %d).StartTime.ToUniversalTime().Ticks.ToString()", os.Getpid()), compatibilityBudget)
	if err != nil {
		t.Fatal("legacy UTC tick compatibility query failed", err)
	}
	if strings.TrimSpace(string(body)) != identity.Started {
		t.Fatal("legacy UTC ticks differ")
	}
	t.Logf("legacy_tick_compatibility elapsed_ms=%d test_budget_ms=%d ticks_match=true", time.Since(compatibilityStarted).Milliseconds(), compatibilityBudget.Milliseconds())
	listener.Close()
	if _, err := readCoreIdentity(context.Background(), controller); err == nil {
		t.Fatal("closed listener accepted")
	}
	childCtx, childCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer childCancel()
	child := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestNativeIdentityChild$")
	child.Env = append(os.Environ(), "ROUTE_AGENT_NATIVE_CHILD_PORT="+strconv.Itoa(port))
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		stdin.Close()
		if !waited {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "ready\n" {
		t.Fatal("owned child failed to bind", err)
	}
	replacement, err := readCoreIdentity(context.Background(), controller)
	if err != nil || replacement.PID != child.Process.Pid || replacement == identity {
		t.Fatal("replacement identity not observed", err)
	}
	if err = (windowsDirectPath{}).CheckOwner(context.Background(), lease); err == nil {
		t.Fatal("old lease accepted replacement listener")
	}
	if exited, err := windowsPublisherExited(context.Background(), child.Process.Pid); err != nil || exited {
		t.Fatal("live child mistaken for absent", err)
	}
	stdin.Close()
	err = child.Wait()
	waited = true
	if err != nil {
		t.Fatal("owned child exit failed", err)
	}
	if exited, err := windowsPublisherExited(context.Background(), child.Process.Pid); err != nil || !exited {
		t.Fatal("exited child not recognized", err)
	}
	if _, err := readCoreIdentity(context.Background(), controller); err == nil {
		t.Fatal("exited listener accepted")
	}
	t.Log("native_checks repeated=10 legacy_ticks_match=true changed_pid_rejected=true changed_start_rejected=true rebind_rejected=true process_exit_verified=true cancellation_verified=true")
}

func TestNativeIdentityChild(t *testing.T) {
	p := os.Getenv("ROUTE_AGENT_NATIVE_CHILD_PORT")
	if p == "" {
		t.Skip("owned helper only")
	}
	port, err := strconv.Atoi(p)
	if err != nil || port < 1 || port > 65535 {
		os.Exit(2)
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		os.Exit(3)
	}
	fmt.Print("ready\n")
	_, _ = io.Copy(io.Discard, os.Stdin)
	listener.Close()
	os.Exit(0)
}
