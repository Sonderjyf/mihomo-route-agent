package route

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRealAcceptanceAPIDiagnosticsNeverExposeCanary(t *testing.T) {
	const canary = "SYNTHETIC_SECRET_CANARY_7dc161af"
	for _, kind := range []string{"transport", "http", "body"} {
		t.Run(kind, func(t *testing.T) {
			j, _ := NewJev(canary, "")
			b := &acceptanceBudget{next: func() http.RoundTripper {
				return acceptanceRoundTrip(func(r *http.Request) (*http.Response, error) {
					if r.Header.Get("Authorization") != "Bearer "+canary {
						t.Fatal("synthetic key missing")
					}
					if kind == "transport" {
						return nil, errors.New(canary)
					}
					status := 401
					if kind == "body" {
						status = 200
					}
					return &http.Response{StatusCode: status, Header: http.Header{"X-Request-Id": []string{canary}}, Body: io.NopCloser(strings.NewReader(canary))}, nil
				})
			}}
			j.client.Transport = b
			_, err := j.Decide(context.Background(), State{Hostname: "example.com"})
			if err == nil || strings.Contains(err.Error(), canary) {
				t.Fatal("missing safe failure")
			}
			wantReason := map[string]string{"transport": "model_transport_failed", "http": "model_http_error", "body": "model_response_invalid"}[kind]
			if acceptanceModelReason(err) != wantReason {
				t.Fatal("lost model failure classification")
			}
			used, records := b.snapshot()
			wire, _ := json.Marshal(records)
			if strings.Contains(string(wire), canary) || used != 1 || len(records) != 1 {
				t.Fatal("unsafe diagnostic")
			}
			if kind == "transport" && (records[0].RequestID != "unknown" || records[0].HTTPStatus != 0 || records[0].Outcome != "transport_failed") {
				t.Fatal("invented response metadata")
			}
			if kind != "transport" && records[0].RequestID != "present" {
				t.Fatal("lost safe presence indicator")
			}
		})
	}
}

func TestRealAcceptanceEarlyFailureReportsKnownZero(t *testing.T) {
	out, err := RunRealAcceptance(context.Background(), 0, "SYNTHETIC_SECRET_CANARY", "SYNTHETIC_SECRET_CANARY")
	if err == nil || out.Stage != "worker_inputs" || out.Reason != "guard_rejected" || out.ModelAttempts != 0 || out.Hosts == nil || out.APIRequests == nil {
		t.Fatal("incomplete early failure report")
	}
	wire, _ := json.Marshal(out)
	if strings.Contains(string(wire), "SYNTHETIC_SECRET_CANARY") {
		t.Fatal("credential in report")
	}
}

type canaryJudge struct{}

func (canaryJudge) Decide(context.Context, State) (Answer, error) {
	return Answer{}, errors.New("SYNTHETIC_SECRET_CANARY")
}
func TestRealAcceptanceNetworkAndJudgeErrorsKeepStage(t *testing.T) {
	p, _ := localProbe(t, true, 200)
	r, err := realHost(context.Background(), 0, "example.com", p, canaryJudge{}, func(context.Context, net.Conn, netip.Addr) error { return nil })
	if err != nil || r.Stage != "model" || r.Reason != "model_unavailable" || r.Accepted != Uncertain {
		t.Fatal("model failure lost stage")
	}
	wire, _ := json.Marshal(r)
	if strings.Contains(string(wire), "SYNTHETIC_SECRET_CANARY") {
		t.Fatal("model error leaked")
	}
	p.pathCheck = func(context.Context, netip.Addr) error { return errors.New("SYNTHETIC_SECRET_CANARY") }
	r, err = realHost(context.Background(), 0, "example.com", p, canaryJudge{}, func(context.Context, net.Conn, netip.Addr) error { return nil })
	if err == nil || r.Stage != "probe" || r.Reason != "guard_rejected" {
		t.Fatal("path guard failure lost stage")
	}
	wire, _ = json.Marshal(r)
	if strings.Contains(string(wire), "SYNTHETIC_SECRET_CANARY") {
		t.Fatal("guard error leaked")
	}
}

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
		if e != nil || lookups != 1 || chains != 1 || j.calls != 1 || r.Accepted != choice || r.AcceptanceReason != "accepted" || r.VLESSTLS != "verified_success" || requests.Load() != 0 {
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
		if e != nil || r.Accepted != Uncertain || r.AcceptanceReason != "not_evaluated" || (!cancel && j.calls != 0) || (cancel && r.Model != "unavailable") {
			t.Fatalf("unsafe acceptance %+v %v", r, e)
		}
	}
}
func TestRealAcceptanceUncertainAndEvidenceVeto(t *testing.T) {
	for _, choice := range []Decision{Proxy, Uncertain} {
		p, _ := localProbe(t, true, 200)
		j := &acceptanceJudge{answer: validAnswer(choice)}
		r, e := realHost(context.Background(), 0, "example.com", p, j, func(context.Context, net.Conn, netip.Addr) error { return nil })
		if e != nil || r.Accepted != Uncertain || r.Choice != choice || r.Model != "answered" || r.AcceptanceReason != "choice_evidence_mismatch" {
			t.Fatalf("lost uncertainty %+v %v", r, e)
		}
	}
}
func TestRealAcceptanceDirectRejectionDiagnostic(t *testing.T) {
	for _, missing := range []bool{false, true} {
		p, _ := localProbe(t, true, 200)
		answer := validAnswer(Direct)
		answer.Probabilities[Direct], answer.Probabilities[Proxy] = .79, .21
		want := "probability_below_threshold"
		if missing {
			answer.Probabilities = nil
			want = "probability_count_invalid"
		}
		j := &acceptanceJudge{answer: answer}
		r, err := realHost(context.Background(), 0, "example.com", p, j, func(context.Context, net.Conn, netip.Addr) error { return nil })
		if err != nil || r.Model != "answered" || r.Choice != Direct || r.Accepted != Uncertain || r.AcceptanceReason != want {
			t.Fatal("lost policy rejection diagnostic")
		}
		wire, _ := json.Marshal(r)
		if strings.Contains(string(wire), "probabilities") || strings.Contains(string(wire), "confidence") || strings.Contains(string(wire), "example.com") {
			t.Fatal("raw model data entered diagnostic")
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
