package route

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func shadowConfig() Config {
	c := testConfig()
	c.Mode, c.Judge = "async", "stub"
	c.LabFixtures = nil
	c.PreflightMS = 2000
	c.Observation = &ObservationConfig{DNS: "127.0.0.1:1", Proxy: "http://127.0.0.1:2", Hosts: []string{"example.com"}, AttemptTimeoutMS: 300}
	return c
}

func TestShadowObserverRunsLocalCollectorWithoutCoreWrites(t *testing.T) {
	for _, stopping := range []string{"drift", "cancel"} {
		t.Run(stopping, func(t *testing.T) {
			c := shadowConfig()
			collector, requests := localProbe(t, false, 200)
			var writes atomic.Int64
			var drift atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes.Add(1)
					w.WriteHeader(405)
					return
				}
				if r.Header.Get("Authorization") != "Bearer synthetic-test-token" {
					t.Error("missing controller authentication")
				}
				switch r.URL.Path {
				case "/configs":
					json.NewEncoder(w).Encode(map[string]string{"mode": "rule"})
				case "/rules":
					target := "BASE"
					if drift.Load() {
						target = "DIRECT"
					}
					json.NewEncoder(w).Encode(map[string]any{"rules": []CoreRule{{Index: 0, Type: "Match", Proxy: target}}})
				case "/connections":
					otherPort := observation("example.com", "Match")
					otherPort.Metadata.DestinationPort = "8443"
					json.NewEncoder(w).Encode(map[string]any{"connections": []CoreConnection{otherPort, observation("not-allowed.test", "Match"), observation("example.com", "Match")}})
				default:
					t.Errorf("unexpected core read %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			c.Controller = server.URL
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			c.HTTPListen = listener.Addr().String()
			listener.Close()
			judge := &recordingJudge{}
			observer, err := NewShadowObserver(c, true, judge, "synthetic-test-token")
			if err != nil {
				t.Fatal(err)
			}
			observer.collector = collector
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- observer.Run(ctx, "") }()
			deadline := time.Now().Add(4 * time.Second)
			for observer.decisions.Load() != 1 && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			observed := observer.decisions.Load()
			recorder := httptest.NewRecorder()
			observer.HTTPHandler().ServeHTTP(recorder, httptest.NewRequest("GET", "/rules/route-agent-tail-proxy.yaml", nil))
			if stopping == "drift" {
				drift.Store(true)
			} else {
				cancel()
			}
			select {
			case err = <-done:
				if (stopping == "drift") != (err != nil) {
					t.Errorf("unexpected %s exit: %v", stopping, err)
				}
			case <-time.After(3 * time.Second):
				cancel()
				<-done
				t.Fatal("observer did not stop on drift")
			}
			if observed != 1 || judge.calls != 1 || observer.attempts.Load() != 1 || observer.commits.Load() != 0 || writes.Load() != 0 || requests.Load() != 0 || recorder.Code != 404 {
				t.Fatalf("shadow violated bounds: decisions=%d judges=%d attempts=%d commits=%d writes=%d target_http=%d provider_http=%d", observed, judge.calls, observer.attempts.Load(), observer.commits.Load(), writes.Load(), requests.Load(), recorder.Code)
			}
			if observer.ready.Load() {
				t.Fatal("still ready after exit")
			}
		})
	}
}

func TestShadowConfigAndPermissionGuards(t *testing.T) {
	c := shadowConfig()
	c.Controller = "http://127.0.0.1:3"
	judge := &recordingJudge{}
	if _, err := NewShadowObserver(c, false, judge, ""); err == nil {
		t.Fatal("missing explicit permission")
	}
	c.Observation.Hosts = []string{"example.com", "example.com"}
	if c.Observation.Validate() == nil {
		t.Fatal("duplicate allowlist")
	}
	c.Observation.Hosts = []string{"private.local"}
	if c.Observation.Validate() == nil {
		t.Fatal("local host")
	}
	c.Observation.Hosts = []string{"example.com"}
	c.LabFixtures = map[string]Evidence{"fixture.test": {}}
	if _, err := NewShadowObserver(c, true, judge, ""); err == nil {
		t.Fatal("mixed real and synthetic evidence")
	}
}
