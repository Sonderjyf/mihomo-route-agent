package route

import (
	"context"
	"encoding/json"
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

type channelDNSWriter struct{ replies chan *dns.Msg }

func (w *channelDNSWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}
}
func (w *channelDNSWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12346}
}
func (w *channelDNSWriter) WriteMsg(m *dns.Msg) error   { w.replies <- m.Copy(); return nil }
func (w *channelDNSWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *channelDNSWriter) Close() error                { return nil }
func (w *channelDNSWriter) TsigStatus() error           { return nil }
func (w *channelDNSWriter) TsigTimersOnly(bool)         {}
func (w *channelDNSWriter) Hijack()                     {}

func TestSameHostDNSWaitsDuringProviderCommit(t *testing.T) {
	var p *Providers
	var armed atomic.Bool
	putStarted, releasePUT := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			if armed.Swap(false) {
				close(putStarted)
				<-releasePUT
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		counts := map[string]any{}
		p.mu.RLock()
		for _, d := range []Decision{Direct, Proxy} {
			counts["learned-"+strings.ToLower(string(d))] = map[string]int{"ruleCount": strings.Count(string(p.served[d]), "  - DOMAIN,")}
		}
		p.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"providers": counts})
	}))
	defer server.Close()
	release := sync.OnceFunc(func() { close(releasePUT) })
	defer release()
	var err error
	p, err = NewProviders(server.URL, "", 32)
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Change(context.Background(), "", nil); err != nil {
		t.Fatal(err)
	}
	c := testConfig()
	c.Mode = "bounded-preflight"
	c.PreflightMS = 3000
	c.DNSDeadlineMS = 5000
	c.Upstream = testUpstream(t)
	c.LabFixtures = map[string]Evidence{"commit-window.route-lab.test": {"repeated_failure", "verified_success"}}
	a, err := New(context.Background(), c, Stub{Answer: validAnswer(Proxy)}, "")
	if err != nil {
		t.Fatal(err)
	}
	a.Providers = p
	d, _ := Normalize("commit-window.route-lab.test")
	armed.Store(true)
	first := make(chan Match, 1)
	go func() { first <- a.Resolve(context.Background(), d) }()
	select {
	case <-putStarted:
	case <-time.After(time.Second):
		t.Fatal("first PUT not reached")
	}
	_, _, ready := p.Status()
	if ready {
		t.Fatal("test did not hold the commit before ACK")
	}
	w := &channelDNSWriter{replies: make(chan *dns.Msg, 1)}
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(d.Host), dns.TypeAAAA)
	go a.ServeDNS(w, q)
	// Wait for an observable join, not a sleep intended to guess scheduling.
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case response := <-w.replies:
			t.Fatalf("same-host AAAA DNS released before provider ACK: rcode=%d", response.Rcode)
		case <-deadline.C:
			t.Fatal("second DNS query did not join the pending decision")
		case <-ticker.C:
			a.pendingMu.Lock()
			pending := a.pending[d.Host]
			joined := pending != nil && pending.waiters == 2
			a.pendingMu.Unlock()
			if joined {
				break waiting
			}
		}
	}
	release()
	select {
	case result := <-first:
		if result.Decision != Proxy {
			t.Error("first judgment did not commit")
		}
	case <-time.After(time.Second):
		t.Error("first Resolve did not complete")
	}
	select {
	case response := <-w.replies:
		if response.Rcode != dns.RcodeSuccess {
			t.Errorf("DNS response after ACK: %v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("second DNS query did not resume after ACK")
	}
	if a.judgments.Load() != 1 || a.commits.Load() != 1 {
		t.Fatalf("shared decision: judgments=%d commits=%d", a.judgments.Load(), a.commits.Load())
	}
}
