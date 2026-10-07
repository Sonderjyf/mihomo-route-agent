package route

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime"
	"time"

	"github.com/miekg/dns"
)

func (a *Agent) exchange(ctx context.Context, q *dns.Msg) (*dns.Msg, error) {
	client := &dns.Client{Net: "udp", Timeout: time.Duration(a.Config.DNSDeadlineMS) * time.Millisecond}
	r, _, err := client.ExchangeContext(ctx, q, a.Config.Upstream)
	if err == nil && r.Truncated {
		client.Net = "tcp"
		r, _, err = client.ExchangeContext(ctx, q, a.Config.Upstream)
	}
	return r, err
}

func protectedAnswer(r *dns.Msg) bool {
	for _, section := range [][]dns.RR{r.Answer, r.Extra} {
		for _, record := range section {
			var ip net.IP
			switch value := record.(type) {
			case *dns.A:
				ip = value.A
			case *dns.AAAA:
				ip = value.AAAA
			}
			if ip != nil {
				addr, ok := netip.AddrFromSlice(ip)
				if !ok || ProtectedIP(addr) {
					return true
				}
			}
		}
	}
	return false
}

func (a *Agent) guardHost(ctx context.Context, host string) (bool, error) {
	type result struct {
		protected bool
		err       error
	}
	results := make(chan result, 2)
	for _, kind := range []uint16{dns.TypeA, dns.TypeAAAA} {
		go func(kind uint16) {
			q := new(dns.Msg)
			q.SetQuestion(dns.Fqdn(host), kind)
			r, err := a.exchange(ctx, q)
			if err == nil && r.Rcode != dns.RcodeSuccess {
				err = fmt.Errorf("hostname did not resolve")
			}
			results <- result{err == nil && protectedAnswer(r), err}
		}(kind)
	}
	protected := false
	var failure error
	for i := 0; i < 2; i++ {
		select {
		case item := <-results:
			protected = protected || item.protected
			if item.err != nil {
				failure = item.err
			}
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	if protected {
		return true, nil
	}
	return false, failure
}

func (a *Agent) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	a.queries.Add(1)
	if q.Opcode != dns.OpcodeQuery || len(q.Question) != 1 || q.Question[0].Qclass != dns.ClassINET {
		r := new(dns.Msg)
		r.SetRcode(q, dns.RcodeFormatError)
		_ = w.WriteMsg(r)
		return
	}
	d, normalizationErr := Normalize(q.Question[0].Name)
	ctx, cancel := context.WithTimeout(a.root, time.Duration(a.Config.DNSDeadlineMS)*time.Millisecond)
	defer cancel()
	r, err := a.exchange(ctx, q)
	if err != nil {
		r = new(dns.Msg)
		r.SetRcode(q, dns.RcodeServerFailure)
		_ = w.WriteMsg(r)
		return
	}
	match := Match{Uncertain, "dns-negative"}
	eligible := q.Question[0].Qtype == dns.TypeA || q.Question[0].Qtype == dns.TypeAAAA || q.Question[0].Qtype == dns.TypeHTTPS
	if r.Rcode == dns.RcodeSuccess && normalizationErr == nil && eligible {
		if protectedAnswer(r) {
			match = Match{Direct, "hard-dns"}
			if _, ok := a.Providers.Lookup(d.Host); ok {
				if err := a.Providers.Change(ctx, d.Host, nil); err != nil {
					r.SetRcode(q, dns.RcodeServerFailure)
				}
			}
		} else {
			match = a.Resolve(ctx, d)
		}
	}
	if ctx.Err() != nil {
		r.SetRcode(q, dns.RcodeServerFailure)
	}
	if normalizationErr != nil {
		match = Match{Uncertain, "unsupported-name"}
	}
	if !eligible {
		match = Match{Uncertain, "unsupported-qtype"}
	}
	if _, udp := w.RemoteAddr().(*net.UDPAddr); udp {
		size := dns.MinMsgSize
		if opt := q.IsEdns0(); opt != nil {
			size = int(opt.UDPSize())
		}
		// Upstream may have retried over TCP. Respect the downstream UDP
		// budget and set TC so that client can retry over TCP as well.
		r.Truncate(size)
	}
	if err = w.WriteMsg(r); err == nil {
		slog.Info("dns_released", "host_id", hostID(d.Host), "qtype", q.Question[0].Qtype, "decision", match.Decision, "source", match.Source)
	}
}

func (a *Agent) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/rules/", a.Providers.Handler)
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		generation, count, ready := a.Providers.Status()
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"mode": a.Config.Mode, "core_ready": ready, "generation": generation, "learned_count": count,
			"queries": a.queries.Load(), "judge_calls": a.judgments.Load(), "commits": a.commits.Load(), "failures": a.failures.Load(), "inflight": a.inflight.Load(), "go_heap_bytes": memory.HeapAlloc})
	})
	return mux
}

// Run binds all loopback listeners before core synchronization so Mihomo can
// download the initial empty providers without a startup dependency cycle.
func (a *Agent) Run(ctx context.Context) error {
	packet, err := net.ListenPacket("udp", a.Config.DNSListen)
	if err != nil {
		return err
	}
	defer packet.Close()
	dnsListener, err := net.Listen("tcp", a.Config.DNSListen)
	if err != nil {
		return err
	}
	defer dnsListener.Close()
	httpListener, err := net.Listen("tcp", a.Config.HTTPListen)
	if err != nil {
		return err
	}
	defer httpListener.Close()
	udp := &dns.Server{PacketConn: packet, Handler: a, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	tcp := &dns.Server{Listener: dnsListener, Handler: a, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	httpServer := &http.Server{Handler: a.HTTPHandler(), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	errors := make(chan error, 3)
	go func() { errors <- udp.ActivateAndServe() }()
	go func() { errors <- tcp.ActivateAndServe() }()
	go func() { errors <- httpServer.Serve(httpListener) }()
	go a.maintain(ctx)
	if a.Config.Controller != "" {
		go func() {
			job, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := a.Providers.Change(job, "", nil); err != nil {
				slog.Warn("core_sync_pending")
			} else {
				slog.Info("core_ready")
			}
		}()
	}
	slog.Info("listeners_ready", "dns", a.Config.DNSListen, "http", a.Config.HTTPListen, "mode", a.Config.Mode)
	select {
	case <-ctx.Done():
	case err = <-errors:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = udp.ShutdownContext(shutdown)
	_ = tcp.ShutdownContext(shutdown)
	_ = httpServer.Shutdown(shutdown)
	return err
}
