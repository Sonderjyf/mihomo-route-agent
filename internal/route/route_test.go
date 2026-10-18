package route

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDomain(t *testing.T) {
	for _, item := range []struct {
		input, host, registered string
		local                   bool
	}{
		{"Example.COM.", "example.com", "example.com", false}, {"api.foo.co.uk", "api.foo.co.uk", "foo.co.uk", false},
		{"例子.中国", "xn--fsqu00a.xn--fiqs8s", "xn--fsqu00a.xn--fiqs8s", false},
		{"192.168.1.2", "192.168.1.2", "", true}, {"[::1]", "::1", "", true}, {"localhost", "localhost", "", true},
		{"printer.local", "printer.local", "", true}, {"router.home.arpa", "router.home.arpa", "", true},
		{"100.100.100.100", "100.100.100.100", "", true},
	} {
		t.Run(item.input, func(t *testing.T) {
			d, err := Normalize(item.input)
			if err != nil || d.Host != item.host || d.Registrable != item.registered || d.Local != item.local {
				t.Fatalf("normalize: %+v, %v", d, err)
			}
		})
	}
	for _, invalid := range []string{"", "bad,name.com", "a..com", "-a.com", "a_.com"} {
		if _, err := Normalize(invalid); err == nil {
			t.Errorf("accepted invalid hostname %q", invalid)
		}
	}
}

func TestPrecedence(t *testing.T) {
	rules := []Rule{{"DOMAIN-SUFFIX", "route-lab.test", Proxy, "trusted"}, {"DOMAIN", "api.route-lab.test", Direct, "manual"}, {"DOMAIN", "service.route-lab.test", Direct, "explicit"}, {"DOMAIN", "localhost", Proxy, "manual"}}
	a, err := New(context.Background(), testConfig(), nil, "")
	if err != nil {
		t.Fatal(err)
	}
	a.Matcher, err = NewMatcher(rules)
	if err != nil {
		t.Fatal(err)
	}
	a.Providers.dirty = false
	a.Providers.entries["api.route-lab.test"] = Entry{Proxy, time.Now().Add(time.Hour)}
	a.Providers.entries["cached.other.test"] = Entry{Proxy, time.Now().Add(time.Hour)}
	for _, item := range []struct {
		host     string
		decision Decision
		source   string
	}{
		{"localhost", Direct, "hard"}, {"api.route-lab.test", Direct, "manual"}, {"service.route-lab.test", Direct, "explicit"},
		{"other.route-lab.test", Proxy, "trusted"}, {"cached.other.test", Proxy, "learned"}, {"new.other.test", Uncertain, "disabled"},
	} {
		d, _ := Normalize(item.host)
		m := a.Resolve(context.Background(), d)
		if m.Decision != item.decision || m.Source != item.source {
			t.Errorf("%s: %+v", item.host, m)
		}
	}
	if _, err := NewMatcher([]Rule{{"PROCESS-NAME", "app.exe", Direct, "trusted"}}); err == nil {
		t.Fatal("unsupported rule pretended to be UNKNOWN")
	}
	output := a.Matcher.MihomoRules("PROXY", Direct)
	manual, learned := -1, -1
	for i, r := range output {
		if r == "DOMAIN,api.route-lab.test,DIRECT" {
			manual = i
		}
		if r == "RULE-SET,learned-proxy,PROXY" {
			learned = i
		}
	}
	if manual < 0 || manual >= learned {
		t.Fatal("rendered precedence differs from matcher")
	}
}

func validAnswer(choice Decision) Answer {
	scores := map[Decision]float64{Direct: 0, Proxy: 0, Uncertain: 0}
	scores[choice] = 1
	return Answer{Type: "choice", Choice: choice, Probabilities: scores}
}

func TestPolicy(t *testing.T) {
	for _, item := range []struct {
		evidence     Evidence
		choice, want Decision
	}{
		{Evidence{"not_tested", "not_tested"}, Direct, Uncertain}, {Evidence{"conflicting_results", "verified_success"}, Proxy, Uncertain},
		{Evidence{"verified_success", "not_tested"}, Direct, Direct}, {Evidence{"repeated_failure", "verified_success"}, Proxy, Proxy},
		{Evidence{"repeated_failure", "not_tested"}, Proxy, Uncertain},
	} {
		if got := Accept(State{Evidence: item.evidence}, validAnswer(item.choice)); got != item.want {
			t.Errorf("evidence %+v: %s", item.evidence, got)
		}
	}
	bad := validAnswer(Direct)
	bad.Probabilities[Direct] = .6
	if Accept(State{Evidence: Evidence{"verified_success", "not_tested"}}, bad) != Uncertain {
		t.Fatal("low score accepted")
	}
	bad = validAnswer(Direct)
	delete(bad.Probabilities, Uncertain)
	if Accept(State{Evidence: Evidence{"verified_success", "not_tested"}}, bad) != Uncertain {
		t.Fatal("invalid score map accepted")
	}
	t.Run("API failure and absolute cancellation", func(t *testing.T) {
		for _, slow := range []bool{false, true} {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("API credential sent to CONNECT proxy")
				}
				if slow {
					time.Sleep(150 * time.Millisecond)
				}
				w.WriteHeader(503)
			}))
			judge, err := NewJev("synthetic-test-key", server.URL)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			start := time.Now()
			_, err = judge.Decide(ctx, State{})
			cancel()
			if err == nil || strings.Contains(err.Error(), "synthetic-test-key") {
				t.Error("failure accepted or key leaked")
			}
			if slow && time.Since(start) > 120*time.Millisecond {
				t.Error("request exceeded absolute context deadline")
			}
			judge.client.CloseIdleConnections()
			server.Close()
		}
	})
}

func testConfig() Config {
	return Config{Mode: "off", Controller: "http://127.0.0.1:1", Upstream: "127.0.0.1:1", PreflightMS: 500, DNSDeadlineMS: 1000, LearnedTTLSeconds: 60, Capacity: 32, MaxAPIRequests: 100, Fallback: Direct}
}

// One small fake controller checks the provider protocol. The real core is
// exercised separately by scripts/smoke_go.py, never in Internet CI.
type controllerFaults struct {
	Reload   atomic.Bool
	Metadata atomic.Bool
}

func testProviders(t *testing.T) (*Providers, *controllerFaults) {
	t.Helper()
	var providers *Providers
	var fail controllerFaults
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			if strings.HasSuffix(r.URL.Path, "learned-proxy") && fail.Reload.Swap(false) {
				w.WriteHeader(500)
			} else {
				w.WriteHeader(204)
			}
			return
		}
		if fail.Metadata.Load() {
			w.WriteHeader(503)
			return
		}
		counts := map[string]any{}
		providers.mu.RLock()
		defer providers.mu.RUnlock()
		for _, decision := range []Decision{Direct, Proxy} {
			count := strings.Count(string(providers.served[decision]), "  - DOMAIN,")
			counts["learned-"+strings.ToLower(string(decision))] = map[string]int{"ruleCount": count}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"providers": counts})
	}))
	t.Cleanup(server.Close)
	var err error
	providers, err = NewProviders(server.URL, "", 32)
	if err != nil {
		t.Fatal(err)
	}
	if err = providers.Change(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	return providers, &fail
}

func testUpstream(t *testing.T) string {
	t.Helper()
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	server := &dns.Server{PacketConn: packet, NotifyStartedFunc: func() { close(started) }, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		r := new(dns.Msg)
		r.SetReply(q)
		if q.Question[0].Qtype == dns.TypeA {
			r.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1}, A: net.ParseIP("198.51.100.7")}}
		}
		_ = w.WriteMsg(r)
	})}
	go server.ActivateAndServe()
	<-started
	t.Cleanup(func() { _ = server.Shutdown() })
	return packet.LocalAddr().String()
}

func TestSingleflightDeadline(t *testing.T) {
	t.Run("same host shares one applied decision", func(t *testing.T) {
		p, _ := testProviders(t)
		c := testConfig()
		c.Mode = "bounded-preflight"
		c.Upstream = testUpstream(t)
		c.LabFixtures = map[string]Evidence{"first.route-lab.test": {"repeated_failure", "verified_success"}}
		var calls atomic.Int32
		judge := Stub{Answer: validAnswer(Proxy), Wait: func(ctx context.Context) error {
			calls.Add(1)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(40 * time.Millisecond):
				return nil
			}
		}}
		a, err := New(context.Background(), c, judge, "")
		if err != nil {
			t.Fatal(err)
		}
		a.Providers = p
		d, _ := Normalize("first.route-lab.test")
		results := make(chan Match, 3)
		for i := 0; i < 3; i++ {
			go func() { results <- a.Resolve(context.Background(), d) }()
		}
		for i := 0; i < 3; i++ {
			if result := <-results; result.Decision != Proxy {
				t.Errorf("decision: %+v", result)
			}
		}
		if calls.Load() != 1 || a.judgments.Load() != 1 {
			t.Fatal("A/AAAA/HTTPS decision was duplicated")
		}
		_ = a.Resolve(context.Background(), d)
		if calls.Load() != 1 {
			t.Fatal("applied cache caused another inference")
		}
	})
	t.Run("caller cancellation and late judge", func(t *testing.T) {
		p, _ := testProviders(t)
		c := testConfig()
		c.Mode = "bounded-preflight"
		c.Upstream = testUpstream(t)
		c.PreflightMS = 30
		c.LabFixtures = map[string]Evidence{"late.route-lab.test": {"repeated_failure", "verified_success"}}
		judge := Stub{Answer: validAnswer(Proxy), Wait: func(context.Context) error { time.Sleep(90 * time.Millisecond); return nil }}
		a, _ := New(context.Background(), c, judge, "")
		a.Providers = p
		d, _ := Normalize("late.route-lab.test")
		start := time.Now()
		m := a.Resolve(context.Background(), d)
		if m.Decision != Uncertain || time.Since(start) > 80*time.Millisecond {
			t.Fatal("late judge blocked preflight")
		}
		time.Sleep(110 * time.Millisecond)
		if _, ok := p.Lookup(d.Host); ok || a.commits.Load() != 0 {
			t.Fatal("late result wrote a rule")
		}
	})
	t.Run("async returns before judge", func(t *testing.T) {
		p, _ := testProviders(t)
		c := testConfig()
		c.Mode = "async"
		c.Upstream = testUpstream(t)
		c.LabFixtures = map[string]Evidence{"async.route-lab.test": {"repeated_failure", "verified_success"}}
		release := make(chan struct{})
		started := make(chan struct{})
		judge := Stub{Answer: validAnswer(Proxy), Wait: func(ctx context.Context) error {
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}}
		a, _ := New(context.Background(), c, judge, "")
		a.Providers = p
		d, _ := Normalize("async.route-lab.test")
		if m := a.Resolve(context.Background(), d); m.Source != "async-pending" {
			t.Fatal(m)
		}
		<-started
		if _, ok := p.Lookup(d.Host); ok {
			t.Fatal("async claimed a rule before commit")
		}
		close(release)
		deadline := time.Now().Add(time.Second)
		for a.commits.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if _, ok := p.Lookup(d.Host); !ok {
			t.Fatal("independent async task failed to apply")
		}
	})
	t.Run("one canceled waiter does not cancel another", func(t *testing.T) {
		p, _ := testProviders(t)
		c := testConfig()
		c.Mode = "bounded-preflight"
		c.Upstream = testUpstream(t)
		c.LabFixtures = map[string]Evidence{"cancel.route-lab.test": {"repeated_failure", "verified_success"}}
		started, release := make(chan struct{}), make(chan struct{})
		judge := Stub{Answer: validAnswer(Proxy), Wait: func(ctx context.Context) error {
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}}
		a, _ := New(context.Background(), c, judge, "")
		a.Providers = p
		d, _ := Normalize("cancel.route-lab.test")
		caller, cancel := context.WithCancel(context.Background())
		first, second := make(chan Match, 1), make(chan Match, 1)
		go func() { first <- a.Resolve(caller, d) }()
		<-started
		go func() { second <- a.Resolve(context.Background(), d) }()
		deadline := time.Now().Add(time.Second)
		for {
			a.pendingMu.Lock()
			waiters := a.pending[d.Host].waiters
			a.pendingMu.Unlock()
			if waiters == 2 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("second waiter did not join")
			}
			time.Sleep(time.Millisecond)
		}
		cancel()
		if m := <-first; m.Decision != Uncertain {
			t.Fatal(m)
		}
		close(release)
		if m := <-second; m.Decision != Proxy {
			t.Fatal("one canceled caller canceled shared work", m)
		}
		if a.judgments.Load() != 1 {
			t.Fatal("duplicated decision after caller cancellation")
		}
	})
	t.Run("last canceled waiter discards late result", func(t *testing.T) {
		p, _ := testProviders(t)
		c := testConfig()
		c.Mode = "bounded-preflight"
		c.Upstream = testUpstream(t)
		c.LabFixtures = map[string]Evidence{"last.route-lab.test": {"repeated_failure", "verified_success"}}
		started, release := make(chan struct{}), make(chan struct{})
		judge := Stub{Answer: validAnswer(Proxy), Wait: func(context.Context) error { close(started); <-release; return nil }}
		a, _ := New(context.Background(), c, judge, "")
		a.Providers = p
		d, _ := Normalize("last.route-lab.test")
		caller, cancel := context.WithCancel(context.Background())
		result := make(chan Match, 1)
		go func() { result <- a.Resolve(caller, d) }()
		<-started
		cancel()
		<-result
		close(release)
		deadline := time.Now().Add(time.Second)
		for a.inflight.Load() != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if _, ok := p.Lookup(d.Host); ok || a.commits.Load() != 0 {
			t.Fatal("late canceled decision was committed")
		}
	})
}

func TestCompilerCommit(t *testing.T) {
	entries := map[string]Entry{"z.route-lab.test": {Proxy, time.Now().Add(time.Hour)}, "a.route-lab.test": {Proxy, time.Now().Add(time.Hour)}}
	compiled := compile(entries)
	if string(compiled[Proxy]) != "payload:\n  - DOMAIN,a.route-lab.test\n  - DOMAIN,z.route-lab.test\n" {
		t.Fatal("compiler must sort exact, unique DOMAIN rules")
	}
	p, fail := testProviders(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			entry := Entry{Proxy, time.Now().Add(time.Hour)}
			if err := p.Change(context.Background(), fmt.Sprintf("host-%d.route-lab.test", i), &entry); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	_, count, ready := p.Status()
	if count != 8 || !ready {
		t.Fatalf("concurrent commits lost entries: count=%d ready=%v", count, ready)
	}
	fail.Reload.Store(true)
	entry := Entry{Direct, time.Now().Add(time.Hour)}
	if p.Change(context.Background(), "failed.route-lab.test", &entry) == nil {
		t.Fatal("partial provider failure marked applied")
	}
	if _, ok := p.Lookup("failed.route-lab.test"); ok {
		t.Fatal("failed generation cached")
	}
	_, count, ready = p.Status()
	if count != 8 || !ready {
		t.Fatal("failed transaction did not restore old snapshot")
	}
	fail.Metadata.Store(true)
	if p.Check(context.Background()) == nil {
		t.Fatal("missing core metadata not detected")
	}
	if _, ok := p.Lookup("host-0.route-lab.test"); ok {
		t.Fatal("used cache while core state was unknown")
	}
	fail.Metadata.Store(false)
	if err := p.Change(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Lookup("host-0.route-lab.test"); !ok {
		t.Fatal("core recovery did not reapply snapshot")
	}
	expired := Entry{Proxy, time.Now().Add(-time.Second)}
	if err := p.Change(context.Background(), "expired.route-lab.test", &expired); err != nil {
		t.Fatal(err)
	}
	if err := p.Change(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	p.mu.RLock()
	body := string(p.served[Proxy])
	p.mu.RUnlock()
	if strings.Contains(body, "expired.route-lab.test") {
		t.Fatal("expired cache entry remained in provider")
	}
}
