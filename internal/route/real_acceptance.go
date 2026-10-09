package route

// Explicit, separately launched acceptance worker. It publishes no routes and
// does not weaken the production collector, ownership, path or Accept guards.
import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

var errAcceptance = errors.New("acceptance_guard_failed")

type RealAPIRequest struct {
	Attempt    int    `json:"attempt"`
	Outcome    string `json:"outcome"`
	HTTPStatus int    `json:"http_status"`
	RequestID  string `json:"request_id_presence"`
}

type RealAcceptanceResult struct {
	Stage          string              `json:"stage"`
	Reason         string              `json:"reason"`
	RouteInventory *RealRouteInventory `json:"route_inventory"`
	APIRequests    []RealAPIRequest    `json:"api_requests"`
	Hosts          []RealHostResult    `json:"hosts"`
	ModelAttempts  int                 `json:"model_attempts"`
	RoutingUpdated bool                `json:"routing_updated"`
	TUNTested      bool                `json:"tun_tested"`
}
type RealHostResult struct {
	Stage    string      `json:"stage"`
	Reason   string      `json:"reason"`
	Index    int         `json:"index"`
	Probe    ProbeReport `json:"probe"`
	VLESSTLS string      `json:"vless_tls"`
	Model    string      `json:"model"`
	Choice   Decision    `json:"choice"`
	Accepted Decision    `json:"accepted"`
}

// Enforce the budget at the actual HTTP transport boundary, including failures.
// A fresh transport per POST avoids transparent reuse retries underneath it.
type acceptanceBudget struct {
	mu      sync.Mutex
	used    int
	next    func() http.RoundTripper
	records []RealAPIRequest
}

func acceptanceTransport(base *http.Transport) *http.Transport {
	t := base.Clone()
	t.DisableKeepAlives = true
	t.ForceAttemptHTTP2 = false
	// HTTP/2 may replay a POST after REFUSED_STREAM/GOAWAY inside a single
	// RoundTrip. Only HTTP/1 on fresh connections makes this attempt cap exact.
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	t.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	return t
}

func (b *acceptanceBudget) RoundTrip(r *http.Request) (*http.Response, error) {
	b.mu.Lock()
	if b.used >= 2 || r.Method != "POST" || r.URL.String() != "https://openrouter.ai/api/alpha/decisions" {
		b.mu.Unlock()
		return nil, errAcceptance
	}
	b.used++
	index := len(b.records)
	b.records = append(b.records, RealAPIRequest{Attempt: b.used, Outcome: "unknown", RequestID: "unknown"})
	b.mu.Unlock()
	t := b.next()
	if c, ok := t.(interface{ CloseIdleConnections() }); ok {
		defer c.CloseIdleConnections()
	}
	response, err := t.RoundTrip(r)
	b.mu.Lock()
	record := &b.records[index]
	if err != nil {
		record.Outcome = "transport_failed"
	} else if response != nil {
		record.Outcome = "http_response"
		if response.StatusCode >= 100 && response.StatusCode <= 599 {
			record.HTTPStatus = response.StatusCode
		}
		record.RequestID = "absent"
		for _, name := range []string{"X-Request-ID", "Request-ID", "X-OpenRouter-Request-ID"} {
			if response.Header.Get(name) != "" {
				record.RequestID = "present"
			}
		}
	}
	b.mu.Unlock()
	return response, err
}

func (b *acceptanceBudget) snapshot() (int, []RealAPIRequest) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, append([]RealAPIRequest{}, b.records...)
}

func acceptanceModelReason(err error) string {
	if err == nil {
		return "model_unavailable"
	}
	// Classify only the existing Jev client's fixed messages. No message or
	// substring is copied into diagnostics, including unknown Judge errors.
	switch err.Error() {
	case "jev transport failed":
		return "model_transport_failed"
	case "invalid jev response size", "invalid jev JSON", "missing route answer":
		return "model_response_invalid"
	}
	if strings.HasPrefix(err.Error(), "jev HTTP ") {
		code, e := strconv.Atoi(strings.TrimPrefix(err.Error(), "jev HTTP "))
		if e == nil && code >= 100 && code <= 599 {
			return "model_http_error"
		}
	}
	return "model_unavailable"
}

func realHost(ctx context.Context, index int, host string, p *TLSCollector, judge Judge, chain func(context.Context, net.Conn, netip.Addr) error) (RealHostResult, error) {
	r := RealHostResult{Stage: "normalize", Reason: "guard_rejected", Index: index, VLESSTLS: "not_tested", Model: "not_attempted", Choice: Uncertain, Accepted: Uncertain}
	r.Probe = ProbeReport{Evidence: Evidence{"not_tested", "not_tested"}, DNS: "not_tested", Direct: []string{}, Proxy: "not_tested"}
	original := p.lookup
	var pinned netip.Addr
	p.lookup = func(ctx context.Context, h string) ([]netip.Addr, error) {
		ips, e := original(ctx, h)
		if e == nil && len(ips) > 0 {
			pinned = ips[0].Unmap()
		}
		return ips, e
	}
	defer func() { p.lookup = original }()
	d, e := Normalize(host)
	if e != nil {
		return r, errAcceptance
	}
	r.Stage = "probe"
	r.Probe, e = p.Collect(ctx, d)
	if e != nil {
		return r, errAcceptance
	}
	if r.Probe.DNS != "resolved" || !pinned.IsValid() || probeProtected(pinned) {
		r.Stage = "dns"
		r.Reason = "dns_unavailable"
		return r, nil
	}
	r.Stage = "vless_tls"
	r.VLESSTLS = p.attemptVerified(ctx, d.Host, net.JoinHostPort(pinned.String(), "443"), true, func(c context.Context, n net.Conn) error { return chain(c, n, pinned) })
	// Explicit transport success never changes the production evidence.
	if r.VLESSTLS != "verified_success" {
		r.Reason = "transport_unverified"
		return r, nil
	}
	r.Stage = "path_guard"
	if p.pathCheck != nil && p.pathCheck(ctx, pinned) != nil {
		return r, errAcceptance
	}
	s := State{Hostname: d.Host, Registrable: d.Registrable, Evidence: r.Probe.Evidence}
	if s.Evidence.DirectTLS != "verified_success" && !(s.Evidence.DirectTLS == "repeated_failure" && s.Evidence.ProxyTLS == "verified_success") {
		r.Stage = "probe"
		r.Reason = "insufficient_evidence"
		return r, nil
	}
	r.Stage = "model"
	modelCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	answer, e := judge.Decide(modelCtx, s)
	if e != nil || modelCtx.Err() != nil {
		r.Model = "unavailable"
		r.Reason = acceptanceModelReason(e)
		if modelCtx.Err() != nil {
			r.Reason = "canceled"
		}
		return r, nil
	}
	if !answer.Choice.Valid() {
		r.Model = "invalid"
		r.Reason = "answer_invalid"
		return r, nil
	}
	r.Model = "answered"
	r.Choice = answer.Choice
	r.Accepted = Accept(s, answer)
	r.Stage = "complete"
	r.Reason = "none"
	return r, nil
}

type acceptanceConnection struct {
	Chains   []string `json:"chains"`
	Rule     string   `json:"rule"`
	Metadata struct {
		SourceIP        string `json:"sourceIP"`
		SourcePort      string `json:"sourcePort"`
		DestinationIP   string `json:"destinationIP"`
		DestinationPort string `json:"destinationPort"`
		Network         string `json:"network"`
	} `json:"metadata"`
}

func acceptanceChain(rows []acceptanceConnection, sourcePort string, target netip.Addr) bool {
	matches := 0
	for _, r := range rows {
		if r.Metadata.SourceIP != "127.0.0.1" || r.Metadata.SourcePort != sourcePort || r.Metadata.DestinationIP != target.String() || r.Metadata.DestinationPort != "443" || r.Metadata.Network != "tcp" {
			continue
		}
		if r.Rule != "Match" || len(r.Chains) != 1 || r.Chains[0] != "acceptance-vless" {
			return false
		}
		matches++
	}
	return matches == 1
}

// RunRealAcceptance is only called by cmd/real-acceptance. The wrapper supplies
// the PID of its own freshly spawned, hash-verified standalone core.
func RunRealAcceptance(ctx context.Context, pid int, key, secret string) (out RealAcceptanceResult, resultErr error) {
	out = RealAcceptanceResult{Stage: "worker_inputs", Reason: "guard_rejected", Hosts: []RealHostResult{}, APIRequests: []RealAPIRequest{}}
	var budget *acceptanceBudget
	defer func() {
		if budget != nil {
			out.ModelAttempts, out.APIRequests = budget.snapshot()
		}
	}()
	const controller = "http://127.0.0.1:19097"
	if pid < 1 || key == "" || secret == "" {
		return out, errAcceptance
	}
	out.Stage = "core_identity"
	identity, e := readCoreIdentity(ctx, controller)
	if e != nil || identity.PID != pid {
		return out, errAcceptance
	}
	checkOwner := func(c context.Context) error {
		now, e := readCoreIdentity(c, controller)
		if e != nil || now != identity {
			return errAcceptance
		}
		return nil
	}
	// Inventory chooses an index; the unchanged production guard validates every
	// actual resolved target before and after direct probing.
	out.Stage = "route_inventory"
	index, inventory, e := acceptanceRouteInventory(ctx, runWindowsQuery)
	out.RouteInventory = &inventory
	if e != nil {
		return out, errAcceptance
	}
	out.Stage = "physical_guard"
	path := windowsDirectPath{index: index}
	if path.Check(ctx, netip.MustParseAddr("1.1.1.1")) != nil {
		return out, errAcceptance
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer transport.CloseIdleConnections()
	get := func(c context.Context, path string, value any) error {
		req, _ := http.NewRequestWithContext(c, "GET", controller+path, nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		res, e := client.Do(req)
		if e != nil {
			return errAcceptance
		}
		defer res.Body.Close()
		body, e := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
		if e != nil || res.StatusCode != 200 || len(body) > 1<<20 || json.Unmarshal(body, value) != nil {
			return errAcceptance
		}
		return nil
	}
	coreCheck := func(c context.Context) error {
		out.Stage = "core_identity"
		if checkOwner(c) != nil {
			return errAcceptance
		}
		var config struct {
			Mode string `json:"mode"`
			Tun  struct {
				Enable bool `json:"enable"`
			} `json:"tun"`
		}
		var rules struct {
			Rules []CoreRule `json:"rules"`
		}
		out.Stage = "controller_contract"
		if get(c, "/configs", &config) != nil || config.Mode != "rule" || config.Tun.Enable || get(c, "/rules", &rules) != nil || len(rules.Rules) != 1 {
			return errAcceptance
		}
		r := rules.Rules[0]
		if r.Type != "Match" || r.Index != 0 || r.Payload != "" || r.Proxy != "acceptance-vless" || r.Extra != nil && r.Extra.Disabled {
			return errAcceptance
		}
		return nil
	}
	if coreCheck(ctx) != nil {
		return out, errAcceptance
	}
	out.Stage = "model_setup"
	judge, e := NewJev(key, "")
	if e != nil {
		return out, errAcceptance
	}
	base := judge.client.Transport.(*http.Transport)
	resolver, e := NewTLSCollector("1.1.1.1:53", "http://127.0.0.1:17897", 5*time.Second)
	if e != nil {
		return out, errAcceptance
	}
	base.DialContext = func(c context.Context, network, address string) (net.Conn, error) {
		if address != "openrouter.ai:443" {
			return nil, errAcceptance
		}
		ips, e := resolver.lookup(c, "openrouter.ai")
		if e != nil || len(ips) == 0 || len(ips) > 16 {
			return nil, errAcceptance
		}
		for _, ip := range ips {
			if probeProtected(ip) {
				return nil, errAcceptance
			}
		}
		if path.Check(c, ips[0]) != nil || checkOwner(c) != nil {
			return nil, errAcceptance
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(c, network, net.JoinHostPort(ips[0].String(), "443"))
	}
	budget = &acceptanceBudget{next: func() http.RoundTripper { return acceptanceTransport(base) }}
	judge.client.Transport = budget
	defer func() { base.CloseIdleConnections() }()
	for i, host := range []string{"example.com", "www.cloudflare.com"} {
		if coreCheck(ctx) != nil {
			return out, errAcceptance
		}
		p, e := NewTLSCollector("1.1.1.1:53", "http://127.0.0.1:17897", 5*time.Second)
		if e != nil {
			return out, errAcceptance
		}
		p.pathCheck = func(c context.Context, ip netip.Addr) error {
			if checkOwner(c) != nil {
				return errAcceptance
			}
			return path.Check(c, ip)
		}
		r, e := realHost(ctx, i, host, p, judge, func(c context.Context, n net.Conn, ip netip.Addr) error {
			if checkOwner(c) != nil {
				return errAcceptance
			}
			_, port, e := net.SplitHostPort(n.LocalAddr().String())
			if e != nil {
				return errAcceptance
			}
			var rows struct {
				Connections []acceptanceConnection `json:"connections"`
			}
			if get(c, "/connections", &rows) != nil || !acceptanceChain(rows.Connections, port, ip) {
				return errAcceptance
			}
			return nil
		})
		out.Hosts = append(out.Hosts, r)
		out.Stage = r.Stage
		if e != nil || coreCheck(ctx) != nil {
			return out, errAcceptance
		}
	}
	out.Stage = "complete"
	out.Reason = "none"
	return out, nil
}
