package route

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func localProbe(t *testing.T, directWorks bool, proxyStatus int) (*TLSCollector, *atomic.Int64) {
	t.Helper()
	var applicationRequests atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { applicationRequests.Add(1) }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	localAddress := strings.TrimPrefix(server.URL, "https://")
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Host != "198.51.100.7:443" {
			t.Error("unexpected proxy request")
			w.WriteHeader(400)
			return
		}
		if proxyStatus != 200 {
			w.WriteHeader(proxyStatus)
			return
		}
		upstream, err := net.Dial("tcp", localAddress)
		if err != nil {
			t.Error(err)
			w.WriteHeader(502)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			t.Error(err)
			return
		}
		fmt.Fprint(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
		go func() {
			defer conn.Close()
			defer upstream.Close()
			go func() { _, _ = io.Copy(upstream, conn); upstream.Close() }()
			_, _ = io.Copy(conn, upstream)
		}()
	}))
	t.Cleanup(proxy.Close)
	// This is a real local DNS server from the existing test helper. Its
	// documentation-range result is pinned then mapped to our own TLS listener.
	p, err := NewTLSCollector(testUpstream(t), proxy.URL, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	p.roots = x509.NewCertPool()
	p.roots.AddCert(server.Certificate())
	dialer := &net.Dialer{}
	p.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == p.proxy {
			return dialer.DialContext(ctx, network, address)
		}
		if address != "198.51.100.7:443" {
			return nil, fmt.Errorf("unexpected nonlocal test destination")
		}
		if !directWorks {
			return nil, errors.New("synthetic direct connect failure")
		}
		return dialer.DialContext(ctx, network, localAddress)
	}
	return p, &applicationRequests
}

func TestTLSCollectorLocalDirectAndProxyPaths(t *testing.T) {
	for _, direct := range []bool{true, false} {
		t.Run(fmt.Sprint(direct), func(t *testing.T) {
			p, applicationRequests := localProbe(t, direct, 200)
			d, _ := Normalize("example.com")
			r, err := p.Collect(context.Background(), d)
			if err != nil {
				t.Fatal(err)
			}
			if direct && (r.Evidence.DirectTLS != "verified_success" || len(r.Direct) != 1 || r.Proxy != "not_tested") {
				t.Fatalf("direct: %+v", r)
			}
			if !direct && (r.Evidence.DirectTLS != "repeated_failure" || r.Evidence.ProxyTLS != "verified_success" || len(r.Direct) != 2) {
				t.Fatalf("proxy: %+v", r)
			}
			if applicationRequests.Load() != 0 {
				t.Fatal("sent application HTTP request")
			}
		})
	}
}

type recordingJudge struct {
	calls int
	state State
}

func (j *recordingJudge) Decide(_ context.Context, s State) (Answer, error) {
	j.calls++
	j.state = s
	return validAnswer(Proxy), nil
}

func TestProbeCertificateAndProxyFailuresRemainUncertain(t *testing.T) {
	for _, kind := range []string{"untrusted_direct", "wrong_hostname_proxy", "proxy_rejected"} {
		t.Run(kind, func(t *testing.T) {
			status := 200
			if kind == "proxy_rejected" {
				status = 503
			}
			p, _ := localProbe(t, kind == "untrusted_direct", status)
			host := "example.com"
			if kind == "untrusted_direct" {
				p.roots = x509.NewCertPool()
			}
			if kind == "wrong_hostname_proxy" {
				host = "wrong.example.test"
			}
			j := &recordingJudge{}
			state, decision, err := EvaluateEvidence(context.Background(), host, p, j)
			if err != nil || decision != Uncertain || j.calls != 0 {
				t.Fatalf("unsafe decision %s %+v err=%v calls=%d", decision, state, err, j.calls)
			}
		})
	}
}

func TestProbeBlocksMixedPrivateFakeIPAndCancellation(t *testing.T) {
	p, _ := localProbe(t, true, 200)
	for _, ip := range []string{"127.0.0.1", "198.19.0.1", "192.168.1.1"} {
		p.lookup = func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("198.51.100.7"), netip.MustParseAddr(ip)}, nil
		}
		p.dial = func(context.Context, string, string) (net.Conn, error) {
			t.Fatal("dialed protected answer")
			return nil, nil
		}
		d, _ := Normalize("example.com")
		r, err := p.Collect(context.Background(), d)
		if err != nil || r.DNS != "protected_or_fake_ip" || len(r.Direct) != 0 {
			t.Fatalf("%+v %v", r, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, _ := Normalize("example.com")
	if _, err := p.Collect(ctx, d); err == nil {
		t.Fatal("ignored cancellation")
	}
}

type localModelTransport func(*http.Request) (*http.Response, error)

func TestProbeCancelsBlockedTLSWithoutModelCall(t *testing.T) {
	p, _ := NewTLSCollector("127.0.0.1:1", "http://127.0.0.1:2", 50*time.Millisecond)
	p.lookup = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("198.51.100.7")}, nil
	}
	p.dial = func(context.Context, string, string) (net.Conn, error) {
		client, peer := net.Pipe()
		t.Cleanup(func() { peer.Close() })
		return client, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	judge := &recordingJudge{}
	start := time.Now()
	_, decision, err := EvaluateEvidence(ctx, "example.com", p, judge)
	if err == nil || decision != Uncertain || judge.calls != 0 || time.Since(start) > time.Second {
		t.Fatalf("unbounded/canceled result: %s %v", decision, err)
	}
}

func (f localModelTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCollectedEvidenceFlowsThroughJevWireAndPolicy(t *testing.T) {
	p, _ := localProbe(t, false, 200)
	calls := 0
	j := &Jev{key: "synthetic-not-a-credential", client: &http.Client{Transport: localModelTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://openrouter.ai/api/alpha/decisions" {
			t.Fatal("unexpected API recipient")
		}
		var body struct {
			Model string `json:"model"`
			State State  `json:"state"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "typesafe/jev-1.13" || body.State.Hostname != "example.com" || body.State.Evidence.DirectTLS != "repeated_failure" || body.State.Evidence.ProxyTLS != "verified_success" {
			t.Fatalf("wrong model state: %+v", body)
		}
		// No network: this in-memory transport is the stub model endpoint.
		wire, _ := json.Marshal(map[string]any{"answers": map[string]Answer{"route": validAnswer(Proxy)}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(wire))), Header: make(http.Header)}, nil
	})}}
	_, decision, err := EvaluateEvidence(context.Background(), "example.com", p, j)
	if err != nil || decision != Proxy || calls != 1 {
		t.Fatalf("decision=%s calls=%d err=%v", decision, calls, err)
	}
}
