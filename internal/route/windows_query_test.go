package route

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
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

func TestCoreIdentityJSONTypesAndMissingFields(t *testing.T) {
	for _, test := range []struct{ body, stage string }{
		{`not json`, "json_invalid"}, {`{"pid":"42","started":"638000000000000000"}`, "json_invalid"},
		{`{"pid":42,"started":638000000000000000}`, "json_invalid"},
		{`{"pid":42}`, "fields_missing"}, {`{"started":"638000000000000000"}`, "fields_missing"},
		{`{"pid":42,"started":"2026-10-08"}`, "start_ticks_invalid"},
		{`{"pid":42,"started":"+638000000000000000"}`, "start_ticks_invalid"},
	} {
		_, err := parseCoreIdentity([]byte(test.body))
		var query *windowsQueryError
		if !errors.As(err, &query) || query.Stage != test.stage {
			t.Fatalf("unexpected parse error: %v", err)
		}
	}
	value, err := parseCoreIdentity([]byte(`{"pid":42,"started":"638000000000000000"}`))
	if err != nil || value.PID != 42 || value.Started != "638000000000000000" {
		t.Fatal("valid decimal ticks rejected", err)
	}
}

// Opt-in ordinary CI diagnostic: queries only this test process's loopback
// listener and public cmdlet metadata. Never starts a core, app, TUN or model.
func TestWindowsOwnedQueryMechanism(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("ROUTE_AGENT_QUERY_DIAGNOSTIC") != "1" {
		t.Skip("explicit Windows owned-process diagnostic")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	// Run the production entry first, before any CIM warm-up, on a fresh CI
	// guest. Unlike the diagnostic comparisons, failure here fails the test.
	started := time.Now()
	identity, err := readCoreIdentity(context.Background(), fmt.Sprintf("http://127.0.0.1:%d", port))
	if err != nil {
		t.Fatal("production owned-listener identity query failed:", err)
	}
	if identity.PID != os.Getpid() || identity.Started == "" {
		t.Fatal("production identity did not match the owned test process")
	}
	first, _ := json.Marshal(map[string]any{"case": "production_first_query", "budget_ms": 12000, "elapsed_ms": time.Since(started).Milliseconds(), "pid_matches_self": true, "decimal_tick_digits": len(identity.Started), "success": true})
	t.Log(string(first))
	for _, budget := range []time.Duration{3 * time.Second, 12 * time.Second} {
		started := time.Now()
		body, queryErr := runWindowsQuery(context.Background(), "owned_listener_diagnostic", coreIdentityScript(port), budget)
		record := map[string]any{"case": "post_production_diagnostic", "budget_ms": budget.Milliseconds(), "elapsed_ms": time.Since(started).Milliseconds(), "success": queryErr == nil}
		if queryErr != nil {
			record["safe_error"] = queryErr.Error()
		} else {
			identity, parseErr := parseCoreIdentity(body)
			if parseErr != nil {
				record["safe_error"] = parseErr.Error()
				record["success"] = false
			} else {
				record["pid_matches_self"] = identity.PID == os.Getpid()
				record["decimal_tick_digits"] = len(identity.Started)
			}
		}
		encoded, _ := json.Marshal(record)
		t.Log(string(encoded))
		// Historical failure is not inferred from this new host. Emit evidence;
		// the successful mechanism check below is separate from production acceptance.
	}
	metadata, err := runWindowsQuery(context.Background(), "cmdlet_metadata", `$a=Get-Command Get-NetAdapter; @{interface_index=$a.Parameters.ContainsKey('InterfaceIndex');physical=$a.Parameters.ContainsKey('Physical')}|ConvertTo-Json -Compress`, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var supported struct {
		Index    bool `json:"interface_index"`
		Physical bool `json:"physical"`
	}
	if json.Unmarshal(metadata, &supported) != nil || !supported.Index || !supported.Physical {
		t.Fatal("adapter parameter contract unsupported")
	}
	t.Log("adapter InterfaceIndex and Physical parameters verified; no route/adapter query was issued")
}
