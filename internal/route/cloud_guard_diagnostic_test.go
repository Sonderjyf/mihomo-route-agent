package route

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"runtime"
	"testing"
	"time"
)

// Explicit cloud-only diagnostic. These cmdlets read local adapter/route tables;
// neither the inventory nor the production guard sends packets to the target.
func TestCloudPhysicalRouteGuardDiagnostic(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("ROUTE_AGENT_CLOUD_GUARD_DIAGNOSTIC") != "1" {
		t.Skip("explicit cloud-only route diagnostic")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" || os.Getenv("GITHUB_REPOSITORY") != "Sonderjyf/mihomo-route-agent" || os.Getenv("GITHUB_RUN_ATTEMPT") != "1" {
		t.Fatal("requires the first attempt on the authorized hosted repository")
	}
	if os.Getenv("OPENROUTER_API_KEY") != "" || os.Getenv("VLESS_NODE_JSON") != "" {
		t.Fatal("diagnostic must not receive real acceptance secrets")
	}
	const inventory = `$ErrorActionPreference='Stop'; $a=@(Get-NetAdapter -Physical); $r=@(Find-NetRoute -RemoteIPAddress '1.1.1.1'); @{physical=@($a | ForEach-Object {@{index=[int]$_.InterfaceIndex;up=($_.Status -eq 'Up');hardware=[bool]$_.HardwareInterface}});route_indices=@($r | ForEach-Object {[int]$_.InterfaceIndex});route_rows=$r.Count}|ConvertTo-Json -Depth 4 -Compress`
	body, err := runWindowsQuery(context.Background(), "cloud_route_inventory", inventory, 15*time.Second)
	if err != nil {
		t.Fatal("read-only inventory unavailable:", err)
	}
	var result struct {
		Physical []struct {
			Index    int  `json:"index"`
			Up       bool `json:"up"`
			Hardware bool `json:"hardware"`
		} `json:"physical"`
		RouteIndices []int `json:"route_indices"`
		RouteRows    int   `json:"route_rows"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.RouteIndices) == 0 || result.RouteIndices[0] < 1 {
		t.Fatal("inventory did not identify the unconstrained selected route")
	}
	// Log only re-encoded typed fields: no MACs, host addresses, names or raw output.
	redacted, _ := json.Marshal(result)
	t.Log("cloud_route_inventory=" + string(redacted))
	index := result.RouteIndices[0]
	started := time.Now()
	err = (windowsDirectPath{index: index}).Check(context.Background(), netip.MustParseAddr("1.1.1.1"))
	guard, _ := json.Marshal(map[string]any{"production_guard_passed": err == nil, "selected_interface_index": index, "elapsed_ms": time.Since(started).Milliseconds(), "budget_ms": 3000, "inventory_precedes_guard": true, "target_packets_sent": false})
	t.Log("cloud_route_guard=" + string(guard))
	if err != nil {
		t.Fatal("unmodified production guard rejected cloud route:", err)
	}
}
