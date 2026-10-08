package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoveryRealProviderAdapter(t *testing.T) {
	for _, scenario := range []string{"expired-success", "watch-crash", "live-publisher", "changed-config", "changed-core", "changed-rules", "legacy-lock", "occupied-port", "missing-fresh-fetch", "changed-lock"} {
		t.Run(scenario, func(t *testing.T) {
			c := shadowConfig()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			c.HTTPListen = listener.Addr().String()
			if scenario == "occupied-port" {
				defer listener.Close()
			} else {
				listener.Close()
			}
			c.ProxyGroup = "LEARNED"
			rules := []CoreRule{{Index: 0, Type: "Domain", Payload: "original.test", Proxy: "BASE"},
				{Index: 1, Type: "AND", Payload: tailCorePayload("route-agent-tail-direct"), Proxy: "DIRECT"},
				{Index: 2, Type: "AND", Payload: tailCorePayload("route-agent-tail-proxy"), Proxy: "LEARNED"},
				{Index: 3, Type: "Match", Proxy: "BASE"}}
			var puts, gets atomic.Int64
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-controller" {
					t.Error("missing controller auth")
				}
				if r.Method == "GET" {
					gets.Add(1)
				}
				switch {
				case r.URL.Path == "/configs":
					json.NewEncoder(w).Encode(map[string]string{"mode": "rule"})
				case r.URL.Path == "/rules":
					actual := append([]CoreRule(nil), rules...)
					if scenario == "changed-rules" {
						actual[0].Payload = "replacement.test"
					}
					json.NewEncoder(w).Encode(map[string]any{"rules": actual})
				case r.Method == "PUT" && (r.URL.Path == "/providers/rules/route-agent-tail-direct" || r.URL.Path == "/providers/rules/route-agent-tail-proxy"):
					puts.Add(1)
					if scenario != "missing-fresh-fetch" {
						transport := http.DefaultTransport.(*http.Transport).Clone()
						transport.Proxy = nil
						client := &http.Client{Transport: transport, Timeout: time.Second}
						resp, e := client.Get("http://" + c.HTTPListen + "/rules/" + strings.TrimPrefix(r.URL.Path, "/providers/rules/") + ".yaml")
						if e != nil {
							t.Error(e)
							w.WriteHeader(500)
							return
						}
						body, _ := io.ReadAll(resp.Body)
						resp.Body.Close()
						transport.CloseIdleConnections()
						if string(body) != "payload: []\n" {
							t.Errorf("recovery published %q", body)
						}
					}
					w.WriteHeader(204)
				case r.URL.Path == "/providers/rules":
					providers := map[string]any{}
					for _, suffix := range []string{"direct", "proxy"} {
						name := "route-agent-tail-" + suffix
						providers[name] = map[string]any{"name": name, "behavior": "Classical", "vehicleType": "HTTP", "ruleCount": 0}
					}
					json.NewEncoder(w).Encode(map[string]any{"providers": providers})
				default:
					t.Errorf("unexpected endpoint (must not observe or reload config): %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer core.Close()
			c.Controller = core.URL
			root := t.TempDir()
			configPath := filepath.Join(root, "effective.yaml")
			ownershipPath := filepath.Join(root, "ownership.json")
			statePath := filepath.Join(root, "state.json")
			write := func(path string, body []byte) {
				t.Helper()
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(configPath, []byte("synthetic config"))
			lease := Ownership{Version: 1, Controller: c.Controller, Listen: c.HTTPListen, ProxyGroup: c.ProxyGroup, ConfigPath: configPath, ConfigSHA256: digest([]byte("synthetic config")), RulesSHA256: rulesDigest(rules), Expires: time.Now().Add(-time.Hour), Token: strings.Repeat("a", 64), CorePID: 123, CoreStarted: "synthetic"}
			data, _ := json.Marshal(lease)
			write(ownershipPath, data)
			lockBody, _ := json.Marshal(publisherLock{Version: 1, PID: 456})
			write(ownershipPath+".lock", lockBody)
			guard := &testPathChecker{}
			if scenario == "changed-core" {
				guard.restarted.Store(true)
			}
			if scenario == "changed-config" {
				write(configPath, []byte("new config"))
			}
			if scenario == "legacy-lock" {
				write(ownershipPath+".lock", nil)
			}
			absent := func(context.Context, int) error {
				if scenario == "live-publisher" {
					return fmt.Errorf("still alive")
				}
				if scenario == "changed-lock" {
					write(ownershipPath+".lock", []byte("replaced"))
				}
				return nil
			}
			recover := func(ctx context.Context) error {
				return recoverControlled(ctx, c, true, ownershipPath, statePath, "synthetic-controller", guard, absent)
			}
			if scenario == "watch-crash" {
				checks := 0
				var recovered bool
				recovered, err = watchPublisher(context.Background(), ownershipPath+".lock", func(context.Context, int) (bool, error) { checks++; return checks > 1, nil }, recover)
				if !recovered || checks != 2 {
					t.Fatal("watcher did not observe one exit transition")
				}
			} else {
				err = recover(context.Background())
			}
			if scenario == "expired-success" || scenario == "watch-crash" {
				if err != nil {
					t.Fatal(err)
				}
				if puts.Load() != 2 {
					t.Fatalf("expected two empty provider updates, got %d", puts.Load())
				}
				body, e := os.ReadFile(statePath)
				var journal observerJournal
				if e != nil || json.Unmarshal(body, &journal) != nil || journal.Phase != "recovered_stopped" || len(journal.Entries) != 0 {
					t.Fatalf("bad stopped journal: %s / %v", body, e)
				}
			} else {
				if err == nil {
					t.Fatal("unsafe recovery accepted")
				}
				if scenario != "missing-fresh-fetch" && puts.Load() != 0 {
					t.Fatal("wrote through failed ownership guard")
				}
			}
			if scenario == "live-publisher" && gets.Load() != 0 {
				t.Fatal("contacted core before verifying publisher exit")
			}
			if guard.calls.Load() != 0 {
				t.Fatal("cleanup performed a direct-path probe")
			}
			if _, err := os.Stat(ownershipPath + ".lock"); err != nil {
				t.Fatal("consumed lease lock removed", err)
			}
			if _, err := os.Stat(ownershipPath + ".recovery.lock"); !os.IsNotExist(err) {
				t.Fatal("recovery mutex leaked", err)
			}
		})
	}
}

func TestRecoveryWatcherStopsWithoutWriting(t *testing.T) {
	for _, scenario := range []string{"clean", "changed", "cancelled", "unknown"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "publisher.lock")
			body, _ := json.Marshal(publisherLock{Version: 1, PID: 456})
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			recovered, err := watchPublisher(ctx, path, func(context.Context, int) (bool, error) {
				calls++
				switch scenario {
				case "clean":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				case "changed":
					if err := os.WriteFile(path, []byte("different"), 0600); err != nil {
						t.Fatal(err)
					}
				case "cancelled":
					cancel()
				case "unknown":
					return false, fmt.Errorf("CIM unavailable")
				}
				return false, nil
			}, func(context.Context) error { t.Fatal("unexpected recovery"); return nil })
			if recovered || calls != 1 {
				t.Fatalf("unexpected result %v calls %d", recovered, calls)
			}
			if (scenario == "clean") != (err == nil) {
				t.Fatalf("unexpected error %v", err)
			}
		})
	}
}
