package route

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRealAcceptanceHTTP1NoHiddenRetry(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.ProtoMajor != 1 {
			t.Error("model transport negotiated HTTP/2")
		}
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			t.Error(e)
			return
		}
		conn.Close()
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	// Trust this loopback server's CA; never disable certificate verification.
	transport := acceptanceTransport(server.Client().Transport.(*http.Transport))
	defer transport.CloseIdleConnections()
	req, _ := http.NewRequest("POST", server.URL, strings.NewReader("offline fixture"))
	if _, e := transport.RoundTrip(req); e == nil {
		t.Fatal("missing injected reset")
	}
	if calls.Load() != 1 {
		t.Fatal("POST replayed within one counted RoundTrip")
	}
}

type acceptanceJudge struct {
	answer Answer
	calls  int
	cancel context.CancelFunc
	state  State
}

func (j *acceptanceJudge) Decide(_ context.Context, s State) (Answer, error) {
	j.calls++
	j.state = s
	if j.cancel != nil {
		j.cancel()
	}
	return j.answer, nil
}
func TestRealAcceptanceSeparatesEvidenceAndTransport(t *testing.T) {
	for _, direct := range []bool{true, false} {
		p, requests := localProbe(t, direct, 200)
		choice := Proxy
		if direct {
			choice = Direct
		}
		j := &acceptanceJudge{answer: validAnswer(choice)}
		lookups := 0
		orig := p.lookup
		p.lookup = func(c context.Context, h string) ([]netip.Addr, error) { lookups++; return orig(c, h) }
		chains := 0
		r, e := realHost(context.Background(), 0, "example.com", p, j, func(_ context.Context, n net.Conn, ip netip.Addr) error {
			chains++
			if ip.String() != "198.51.100.7" || n.LocalAddr() == nil {
				t.Fatal("target not pinned")
			}
			return nil
		})
		if e != nil || lookups != 1 || chains != 1 || j.calls != 1 || r.Accepted != choice || r.VLESSTLS != "verified_success" || requests.Load() != 0 {
			t.Fatalf("unexpected stage result: %+v %v", r, e)
		}
		if direct && (r.Probe.Proxy != "not_tested" || j.state.Evidence.ProxyTLS != "not_tested") {
			t.Fatal("explicit proxy overwrote direct evidence")
		}
	}
}
func TestRealAcceptanceChainFailureAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		p, _ := localProbe(t, true, 200)
		ctx, stop := context.WithCancel(context.Background())
		j := &acceptanceJudge{answer: validAnswer(Direct)}
		if cancel {
			j.cancel = stop
		}
		r, e := realHost(ctx, 0, "example.com", p, j, func(context.Context, net.Conn, netip.Addr) error {
			if !cancel {
				return errors.New("test chain mismatch")
			}
			return nil
		})
		stop()
		if e != nil || r.Accepted != Uncertain || (!cancel && j.calls != 0) || (cancel && r.Model != "unavailable") {
			t.Fatalf("unsafe acceptance %+v %v", r, e)
		}
	}
}
func TestRealAcceptanceUncertainAndEvidenceVeto(t *testing.T) {
	for _, choice := range []Decision{Proxy, Uncertain} {
		p, _ := localProbe(t, true, 200)
		j := &acceptanceJudge{answer: validAnswer(choice)}
		r, e := realHost(context.Background(), 0, "example.com", p, j, func(context.Context, net.Conn, netip.Addr) error { return nil })
		if e != nil || r.Accepted != Uncertain || r.Choice != choice || r.Model != "answered" {
			t.Fatalf("lost uncertainty %+v %v", r, e)
		}
	}
}
func TestRealAcceptanceConnectionAssociation(t *testing.T) {
	var row acceptanceConnection
	row.Chains = []string{"acceptance-vless"}
	row.Rule = "Match"
	row.Metadata.SourceIP = "127.0.0.1"
	row.Metadata.SourcePort = "12345"
	row.Metadata.DestinationIP = "198.51.100.7"
	row.Metadata.DestinationPort = "443"
	row.Metadata.Network = "tcp"
	ip := netip.MustParseAddr("198.51.100.7")
	if !acceptanceChain([]acceptanceConnection{row}, "12345", ip) {
		t.Fatal("valid association rejected")
	}
	if acceptanceChain([]acceptanceConnection{row, row}, "12345", ip) || acceptanceChain([]acceptanceConnection{row}, "12346", ip) {
		t.Fatal("ambiguous or foreign connection accepted")
	}
	row.Chains = []string{"DIRECT"}
	if acceptanceChain([]acceptanceConnection{row}, "12345", ip) {
		t.Fatal("direct chain accepted")
	}
}

type acceptanceRoundTrip func(*http.Request) (*http.Response, error)

func (f acceptanceRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestRealAcceptanceHTTPBudgetCountsFailedAttempts(t *testing.T) {
	calls := 0
	b := &acceptanceBudget{next: func() http.RoundTripper {
		return acceptanceRoundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("offline failure") })
	}}
	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/alpha/decisions", nil)
	for i := 0; i < 5; i++ {
		_, _ = b.RoundTrip(req)
	}
	if calls != 2 || b.used != 2 {
		t.Fatal("failed requests exceeded budget")
	}
	bad, _ := http.NewRequest("GET", "https://example.com/", nil)
	b.used = 0
	_, _ = b.RoundTrip(bad)
	if b.used != 0 {
		t.Fatal("unapproved request sent")
	}
}
