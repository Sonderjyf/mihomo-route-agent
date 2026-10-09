package route

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type ProbeReport struct {
	Evidence Evidence `json:"evidence"`
	DNS      string   `json:"dns"`
	Direct   []string `json:"direct_attempts"`
	Proxy    string   `json:"proxy_attempt"`
	timing   probeTiming
	stage    string
	target   netip.Addr
}

type probeTiming struct{ DNSMS, PathMS, DirectTLSMS, ProxyTLSMS int64 }

type EvidenceCollector interface {
	Collect(context.Context, Domain) (ProbeReport, error)
}

// TLSCollector performs only TLS handshakes to port 443, never HTTP requests
// to the destination. Test-only dial/CA overrides are deliberately unexported.
type TLSCollector struct {
	lookup    func(context.Context, string) ([]netip.Addr, error)
	dial      func(context.Context, string, string) (net.Conn, error)
	roots     *x509.CertPool
	proxy     string
	timeout   time.Duration
	pathCheck func(context.Context, netip.Addr) error
}

func NewTLSCollector(dnsAddress, proxyURL string, attemptTimeout time.Duration) (*TLSCollector, error) {
	dnsEndpoint, err := netip.ParseAddrPort(dnsAddress)
	if err != nil || dnsEndpoint.Port() == 0 || attemptTimeout < 50*time.Millisecond || attemptTimeout > 5*time.Second {
		return nil, fmt.Errorf("probe requires explicit numeric DNS endpoint and 50ms..5s attempt timeout")
	}
	if proxyURL == "" || validateController(proxyURL) != nil {
		return nil, fmt.Errorf("probe requires explicit http://127.0.0.1:PORT CONNECT proxy")
	}
	dialer := &net.Dialer{Timeout: attemptTimeout}
	return &TLSCollector{lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
		var addresses []netip.Addr
		for _, kind := range []uint16{dns.TypeA, dns.TypeAAAA} {
			query := new(dns.Msg)
			query.SetQuestion(dns.Fqdn(host), kind)
			client := &dns.Client{Net: "udp", Timeout: attemptTimeout}
			answer, _, err := client.ExchangeContext(ctx, query, dnsEndpoint.String())
			if err == nil && answer.Truncated {
				client.Net = "tcp"
				answer, _, err = client.ExchangeContext(ctx, query, dnsEndpoint.String())
			}
			if err != nil || answer.Rcode != dns.RcodeSuccess {
				return nil, fmt.Errorf("probe DNS lookup unavailable")
			}
			for _, rr := range answer.Answer {
				var ip net.IP
				switch record := rr.(type) {
				case *dns.A:
					ip = record.A
				case *dns.AAAA:
					ip = record.AAAA
				}
				if ip != nil {
					if addr, ok := netip.AddrFromSlice(ip); ok {
						addresses = append(addresses, addr.Unmap())
					}
				}
			}
		}
		return addresses, nil
	}, dial: dialer.DialContext, proxy: strings.TrimPrefix(proxyURL, "http://"), timeout: attemptTimeout}, nil
}

func probeProtected(ip netip.Addr) bool {
	return ProtectedIP(ip) || netip.MustParsePrefix("198.18.0.0/15").Contains(ip.Unmap())
}

func (p *TLSCollector) Collect(ctx context.Context, d Domain) (r ProbeReport, resultErr error) {
	r = ProbeReport{Evidence: Evidence{"not_tested", "not_tested"}, DNS: "not_tested", Direct: []string{}, Proxy: "not_tested"}
	r.stage = "normalize"
	normalized, err := Normalize(d.Host)
	if err != nil || normalized.Host != d.Host || normalized.IP || normalized.Local {
		return r, fmt.Errorf("probe requires a normalized nonlocal hostname")
	}
	budget := 4 * p.timeout
	if p.pathCheck != nil {
		budget += 6 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	dnsCtx, dnsCancel := context.WithTimeout(ctx, p.timeout)
	r.stage = "dns"
	started := time.Now()
	addresses, err := p.lookup(dnsCtx, d.Host)
	r.timing.DNSMS = boundedMS(time.Since(started))
	dnsCancel()
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	if err != nil || len(addresses) == 0 || len(addresses) > 16 {
		r.DNS = "unavailable"
		return r, nil
	}
	for _, ip := range addresses {
		if probeProtected(ip) {
			r.DNS = "protected_or_fake_ip"
			return r, nil
		}
	}
	r.DNS = "resolved"
	r.target = addresses[0].Unmap()
	if p.pathCheck != nil {
		r.stage = "path"
		started = time.Now()
		if err := p.pathCheck(ctx, addresses[0].Unmap()); err != nil {
			r.timing.PathMS += boundedMS(time.Since(started))
			return r, err
		}
		r.timing.PathMS += boundedMS(time.Since(started))
		defer func() {
			started := time.Now()
			if err := p.pathCheck(ctx, addresses[0].Unmap()); err != nil {
				r.stage = "path"
				r.Evidence = Evidence{"not_tested", "not_tested"}
				resultErr = err
			}
			r.timing.PathMS += boundedMS(time.Since(started))
		}()
	}
	// Pin one DNS result across the two direct attempts. This is deliberately
	// not a claim about every address, network or application behind the host.
	address := net.JoinHostPort(addresses[0].Unmap().String(), "443")
	for i := 0; i < 2; i++ {
		r.stage = "direct_tls"
		started = time.Now()
		outcome := p.attempt(ctx, d.Host, address, false)
		r.timing.DirectTLSMS += boundedMS(time.Since(started))
		r.Direct = append(r.Direct, outcome)
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
		if outcome == "verified_success" {
			r.Evidence.DirectTLS = "verified_success"
			return r, nil
		}
		if outcome != "connect_error" && outcome != "timeout" {
			return r, nil
		}
	}
	r.Evidence.DirectTLS = "repeated_failure"
	// Keep the same numeric target through CONNECT: do not let proxy-side DNS
	// silently choose a different or protected address. TLS still verifies host.
	r.stage = "proxy_tls"
	started = time.Now()
	r.Proxy = p.attempt(ctx, d.Host, address, true)
	r.timing.ProxyTLSMS = boundedMS(time.Since(started))
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	if r.Proxy == "verified_success" {
		r.Evidence.ProxyTLS = "verified_success"
	}
	return r, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c bufferedConn) Read(b []byte) (int, error) { return c.reader.Read(b) }

func (p *TLSCollector) attempt(parent context.Context, host, target string, proxy bool) string {
	return p.attemptVerified(parent, host, target, proxy, nil)
}

// The optional observer runs while the verified TLS connection remains open.
// It cannot change the collector's evidence or certificate validation policy.
func (p *TLSCollector) attemptVerified(parent context.Context, host, target string, proxy bool, observe func(context.Context, net.Conn) error) string {
	ctx, cancel := context.WithTimeout(parent, p.timeout)
	defer cancel()
	address := target
	if proxy {
		address = p.proxy
	}
	conn, err := p.dial(ctx, "tcp", address)
	if err != nil {
		var timeout net.Error
		if ctx.Err() != nil || errors.As(err, &timeout) && timeout.Timeout() {
			return "timeout"
		}
		if proxy {
			return "proxy_connect_error"
		}
		return "connect_error"
	}
	defer conn.Close()
	baseConn := conn // immutable capture: cancellation can race CONNECT wrapping
	stop := context.AfterFunc(ctx, func() { _ = baseConn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	if proxy {
		if _, err = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target); err != nil {
			return "proxy_connect_error"
		}
		reader := bufio.NewReaderSize(conn, 4096)
		var header strings.Builder
		for {
			line, e := reader.ReadSlice('\n')
			if e != nil || header.Len()+len(line) > 16384 {
				if ctx.Err() != nil {
					return "timeout"
				}
				return "proxy_protocol_error"
			}
			header.Write(line)
			if string(line) == "\r\n" {
				break
			}
		}
		response, e := http.ReadResponse(bufio.NewReader(strings.NewReader(header.String())), &http.Request{Method: http.MethodConnect})
		if e != nil {
			return "proxy_protocol_error"
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return "proxy_rejected"
		}
		conn = bufferedConn{conn, reader}
	}
	secure := tls.Client(conn, &tls.Config{ServerName: host, RootCAs: p.roots, MinVersion: tls.VersionTLS12})
	err = secure.HandshakeContext(ctx)
	if err == nil {
		if observe != nil && observe(ctx, baseConn) != nil {
			return "chain_unverified"
		}
		return "verified_success"
	}
	if ctx.Err() != nil {
		return "timeout"
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return "certificate_error"
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return "timeout"
	}
	return "tls_error"
}

// EvaluateEvidence wires the collector to any Judge, including NewJev. It
// sends only State's normalized name and coarse evidence, never probe errors,
// resolved addresses or certificates. No CLI/runtime starts this implicitly.
func EvaluateEvidence(ctx context.Context, host string, collector EvidenceCollector, judge Judge) (State, Decision, error) {
	result, err := EvaluateEvidenceDetailed(ctx, host, collector, judge)
	return result.State, result.Diagnostic.Decision, err
}
