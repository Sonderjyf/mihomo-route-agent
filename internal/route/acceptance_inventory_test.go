package route

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAcceptanceInventoryRejectBranchesAndRedaction(t *testing.T) {
	const canary = "SYNTHETIC_SECRET_CANARY_private_address"
	for _, tc := range []struct {
		name, output, reason string
		err                  error
		index                int
	}{
		{"success", "14", "none", nil, 14},
		{"empty", "", "index_not_integer", nil, 0},
		{"text", canary, "index_not_integer", nil, 0},
		{"extra_output", "14\n" + canary, "index_not_integer", nil, 0},
		{"overflow", strings.Repeat("9", 100), "index_not_integer", nil, 0},
		{"zero", "0", "index_not_positive", nil, 0},
		{"negative", "-1", "index_not_positive", nil, 0},
		{"start", "", "query_start_failed", queryFailure(canary, "start_failed", -1, false), 0},
		{"timeout", "", "query_timeout", queryFailure(canary, "timeout", -1, true), 0},
		{"cancel", "", "query_canceled", queryFailure(canary, "canceled", -1, false), 0},
		{"io", "", "query_io_failed", queryFailure(canary, "io_failed", -1, false), 0},
		{"large", "", "query_output_limit", queryFailure(canary, "output_limit", 0, false), 0},
		{"shape", "", "route_shape_rejected", queryFailure(canary, "exit_failed", 7, false), 0},
		{"exit", "", "query_exit_failed", queryFailure(canary, "exit_failed", 1, true), 0},
		{"unknown", "", "unexpected_query_error", errors.New(canary), 0},
		{"unknown_stage", "", "unexpected_query_error", queryFailure(canary, canary, -1, false), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, diagnostic, err := acceptanceRouteInventory(context.Background(), func(_ context.Context, op, script string, budget time.Duration) ([]byte, error) {
				if op != "acceptance_route_inventory" || script != acceptanceInventoryScript || budget != 15*time.Second {
					t.Fatal("query or budget changed")
				}
				return []byte(tc.output), tc.err
			})
			if index != tc.index || diagnostic.Reason != tc.reason || (err == nil) != (tc.reason == "none") || diagnostic.ElapsedMS < 0 || diagnostic.BudgetMS != 15000 {
				t.Fatal("incorrect safe diagnostic")
			}
			if tc.err == nil && (diagnostic.QueryExitCode == nil || *diagnostic.QueryExitCode != 0) {
				t.Fatal("successful query exit not recorded")
			}
			if tc.name == "timeout" && (diagnostic.QueryExitCode != nil || diagnostic.StderrPresent == nil || !*diagnostic.StderrPresent) {
				t.Fatal("invented exit or lost stderr presence")
			}
			wire, _ := json.Marshal(diagnostic)
			if strings.Contains(string(wire), canary) || strings.Contains(string(wire), "1.1.1.1") {
				t.Fatal("private query data leaked")
			}
		})
	}
}

// Called only by the hosted, secret-free Python environment comparison. Reads
// local routing/adapter tables; never sends packets or changes network state.
func TestAcceptanceInventoryEnvironmentChild(t *testing.T) {
	if os.Getenv("ROUTE_AGENT_INVENTORY_CHILD") != "1" {
		t.Skip("explicit secret-free environment diagnostic")
	}
	if runtime.GOOS != "windows" || os.Getenv("OPENROUTER_API_KEY") != "" || os.Getenv("VLESS_NODE_JSON") != "" || os.Getenv("REAL_CONTROLLER_SECRET") != "" {
		os.Exit(2)
	}
	index, diagnostic, err := acceptanceRouteInventory(context.Background(), runWindowsQuery)
	guard, elapsed := "not_run", int64(0)
	if err == nil {
		started := time.Now()
		err = (windowsDirectPath{index: index}).Check(context.Background(), netip.MustParseAddr("1.1.1.1"))
		elapsed = time.Since(started).Milliseconds()
		guard = "passed"
		if err != nil {
			guard = "rejected"
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"inventory": diagnostic, "physical_guard": guard, "physical_guard_elapsed_ms": elapsed, "physical_guard_budget_ms": 3000, "target_packets_sent": false})
	os.Exit(0)
}
