package route

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func observation(host, rule string) CoreConnection {
	c := CoreConnection{ID: "synthetic-id", Start: time.Now(), Rule: rule, Chains: []string{"hop", "BASE"}}
	c.Metadata.Host = host
	c.Metadata.Network = "tcp"
	c.Metadata.DestinationPort = "443"
	return c
}

func TestFallbackEvidenceRejectsAmbiguity(t *testing.T) {
	since := time.Now().Add(-time.Second)
	good := observation("one.route-lab.test", "Match")
	if _, ok := fallbackHost(good, since, "BASE"); !ok {
		t.Fatal("rejected actual MATCH")
	}
	for _, mutate := range []func(*CoreConnection){
		func(c *CoreConnection) { c.Rule = "Domain" }, func(c *CoreConnection) { c.Rule = "RuleSet" },
		func(c *CoreConnection) { c.Rule = "" }, func(c *CoreConnection) { c.Payload = "nested" },
		func(c *CoreConnection) { c.Start = since }, func(c *CoreConnection) { c.Start = time.Now().Add(time.Hour) },
		func(c *CoreConnection) { c.Metadata.Host = "" }, func(c *CoreConnection) { c.Metadata.Host = "192.0.2.1" },
		func(c *CoreConnection) { c.Metadata.Host = "private.local" }, func(c *CoreConnection) { c.Metadata.Network = "udp" },
		func(c *CoreConnection) { c.Chains = []string{"other"} }, func(c *CoreConnection) { c.ID = "" },
		func(c *CoreConnection) { c.Metadata.SpecialRules = "other" },
		func(c *CoreConnection) { c.Metadata.SpecialProxy = "other" },
		func(c *CoreConnection) { c.Metadata.DestinationPort = "8443" },
		func(c *CoreConnection) { c.Metadata.DestinationPort = "" },
		func(c *CoreConnection) { c.Metadata.SniffHost = "different.test" },
	} {
		bad := good
		mutate(&bad)
		if _, ok := fallbackHost(bad, since, "BASE"); ok {
			t.Fatal("accepted ambiguous connection")
		}
	}
}

func TestLabObserverUsesCoreFallbackAndBoundedSyntheticEvidence(t *testing.T) {
	c := testConfig()
	c.HTTPListen = "127.0.0.1:1" // constructor validation only; this test never binds it
	c.Mode, c.Judge, c.ProxyGroup = "async", "stub", "LEARNED"
	c.LabFixtures = map[string]Evidence{
		"one.route-lab.test":     {"repeated_failure", "verified_success"},
		"known.route-lab.test":   {"repeated_failure", "verified_success"},
		"unknown.route-lab.test": {"not_tested", "not_tested"},
	}
	var observer *LabObserver
	var drift atomic.Bool
	var driftOnPut atomic.Bool
	var puts atomic.Int64
	rules := []CoreRule{{Index: 0, Type: "Domain", Payload: "known.route-lab.test", Proxy: "BASE"}, {Index: 1, Type: "AND", Payload: tailCorePayload("route-agent-tail-direct"), Proxy: "DIRECT"}, {Index: 2, Type: "AND", Payload: tailCorePayload("route-agent-tail-proxy"), Proxy: "LEARNED"}, {Index: 3, Type: "Match", Proxy: "BASE"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/configs":
			json.NewEncoder(w).Encode(map[string]string{"mode": "rule"})
		case r.URL.Path == "/rules":
			copyRules := append([]CoreRule{}, rules...)
			if drift.Load() {
				copyRules[0].Proxy = "DIRECT"
			}
			json.NewEncoder(w).Encode(map[string]any{"rules": copyRules})
		case r.URL.Path == "/connections":
			json.NewEncoder(w).Encode(map[string]any{"connections": []CoreConnection{observation("one.route-lab.test", "Match"), observation("one.route-lab.test", "Match"), observation("known.route-lab.test", "Domain"), observation("unknown.route-lab.test", "Match"), observation("not-allowlisted.test", "Match")}})
		case r.Method == "PUT":
			if driftOnPut.Load() {
				drift.Store(true)
			}
			puts.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/providers/rules":
			observer.Providers.mu.RLock()
			count := strings.Count(string(observer.Providers.served[Proxy]), "  - DOMAIN,")
			observer.Providers.mu.RUnlock()
			json.NewEncoder(w).Encode(map[string]any{"providers": map[string]any{"route-agent-tail-direct": map[string]int{"ruleCount": 0}, "route-agent-tail-proxy": map[string]int{"ruleCount": count}}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c.Controller = server.URL
	var err error
	observer, err = NewLabObserver(c, true)
	if err != nil {
		t.Fatal(err)
	}
	observer.baseline = rules
	observer.started = time.Now().Add(-time.Second)
	if err := observer.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if observer.judgments.Load() != 2 || observer.commits.Load() != 1 || puts.Load() != 2 {
		t.Fatal("wrong judgment/commit counts")
	}
	if _, ok := observer.Providers.Lookup("one.route-lab.test"); !ok {
		t.Fatal("missing learned result")
	}
	if err := observer.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if observer.judgments.Load() != 2 {
		t.Fatal("duplicate judgment")
	}
	drift.Store(true)
	if err := observer.poll(context.Background()); err == nil {
		t.Fatal("accepted changed core rules")
	}
	if puts.Load() != 2 {
		t.Fatal("wrote after drift")
	}
	// Even a change during judgment must be detected before provider mutation.
	drift.Store(false)
	observer.attempted = map[string]bool{}
	observer.judge = Stub{Answer: validAnswer(Proxy), Wait: func(context.Context) error { drift.Store(true); return nil }}
	if err := observer.poll(context.Background()); err == nil {
		t.Fatal("missed drift during judgment")
	}
	if puts.Load() != 2 {
		t.Fatal("committed stale decision")
	}
	// A canceled late result cannot be published.
	drift.Store(false)
	observer.attempted = map[string]bool{}
	observer.c.PreflightMS = 5
	observer.judge = Stub{Answer: validAnswer(Proxy), Wait: func(ctx context.Context) error { <-ctx.Done(); return nil }}
	if err := observer.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if puts.Load() != 2 {
		t.Fatal("published timed-out decision")
	}
	observer.c.PreflightMS = 500
	observer.attempted = map[string]bool{}
	observer.judge = Stub{Answer: validAnswer(Proxy)}
	driftOnPut.Store(true)
	if err := observer.poll(context.Background()); err == nil {
		t.Fatal("missed rule drift during provider PUT")
	}
	if observer.commits.Load() != 1 {
		t.Fatal("trusted a commit across rule drift")
	}
}

func TestObserverRequiresExplicitLabMode(t *testing.T) {
	c := testConfig()
	c.Mode = "async"
	c.Judge = "stub"
	c.LabFixtures = map[string]Evidence{"one.route-lab.test": {}}
	if _, err := NewLabObserver(c, false); err == nil {
		t.Fatal("missing lab guard")
	}
	c.Judge = "jev"
	if _, err := NewLabObserver(c, true); err == nil {
		t.Fatal("real model enabled")
	}
}
