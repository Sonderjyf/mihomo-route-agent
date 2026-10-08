package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
)

type CoreRule struct {
	Index   int    `json:"index"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
	Extra   *struct {
		Disabled bool `json:"disabled"`
	} `json:"extra,omitempty"`
}

type CoreConnection struct {
	ID       string    `json:"id"`
	Start    time.Time `json:"start"`
	Rule     string    `json:"rule"`
	Payload  string    `json:"rulePayload"`
	Chains   []string  `json:"chains"`
	Metadata struct {
		Host            string `json:"host"`
		Network         string `json:"network"`
		DestinationPort string `json:"destinationPort"`
		SpecialRules    string `json:"specialRules"`
		SpecialProxy    string `json:"specialProxy"`
		SniffHost       string `json:"sniffHost"`
	} `json:"metadata"`
}

// LabObserver shares one worker across lab, shadow and controlled modes.
// Run owns a single bounded worker; HTTP handlers only read atomic statistics.
type LabObserver struct {
	c         Config
	Providers *Providers
	judge     Judge
	baseline  []CoreRule
	started   time.Time
	attempted map[string]bool
	ready     atomic.Bool
	judgments atomic.Uint64
	commits   atomic.Uint64
	failures  atomic.Uint64
	statePath string
	shadow    bool
	collector EvidenceCollector
	attempts  atomic.Uint64
	decisions atomic.Uint64
	control   *publicationControl
}

func NewLabObserver(c Config, allowLab bool) (*LabObserver, error) {
	if !allowLab || c.Mode != "async" || c.Judge != "stub" || c.Controller == "" || len(c.LabFixtures) == 0 || len(c.LabFixtures) > 128 || len(c.LabFixtures) > c.Capacity {
		return nil, fmt.Errorf("observe-lab requires explicit lab permission, async/stub, isolated controller and 1..128 bounded synthetic fixtures")
	}
	for host := range c.LabFixtures {
		d, err := Normalize(host)
		if err != nil || d.Host != host || d.Local || d.IP || !strings.HasSuffix(host, ".test") {
			return nil, fmt.Errorf("observer fixtures must be normalized .test hostnames")
		}
	}
	return newObserver(c, Stub{Answer: Answer{Type: "choice", Choice: Proxy, Probabilities: map[Decision]float64{Direct: 0, Proxy: 1, Uncertain: 0}}})
}

func newObserver(c Config, judge Judge) (*LabObserver, error) {
	listen, err := netip.ParseAddrPort(c.HTTPListen)
	if err != nil || listen.Addr().String() != "127.0.0.1" || listen.Port() == 0 {
		return nil, fmt.Errorf("observer HTTP listener must use fixed loopback port")
	}
	p, err := newProviders(c.Controller, "", c.Capacity, "route-agent-tail-")
	if err != nil {
		return nil, err
	}
	return &LabObserver{c: c, Providers: p, attempted: map[string]bool{}, judge: judge}, nil
}

func (o *LabObserver) get(ctx context.Context, path string, value any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, o.c.Controller+path, nil)
	if o.Providers.secret != "" {
		req.Header.Set("Authorization", "Bearer "+o.Providers.secret)
	}
	res, err := o.Providers.client.Do(req)
	if err != nil {
		return fmt.Errorf("isolated observation request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("isolated observation HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, value) != nil {
		return fmt.Errorf("invalid or oversized core observation")
	}
	return nil
}

func (o *LabObserver) rules(ctx context.Context) ([]CoreRule, error) {
	var config struct {
		Mode string `json:"mode"`
	}
	if err := o.get(ctx, "/configs", &config); err != nil {
		return nil, err
	}
	if config.Mode != "rule" {
		return nil, fmt.Errorf("isolated core must remain in rule mode")
	}
	var response struct {
		Rules []CoreRule `json:"rules"`
	}
	if err := o.get(ctx, "/rules", &response); err != nil {
		return nil, err
	}
	rules := response.Rules
	if len(rules) == 0 || !o.shadow && len(rules) < 3 {
		return nil, fmt.Errorf("missing tail rules")
	}
	for i, r := range rules {
		known := safeCategory(r.Type, "Domain DomainSuffix DomainKeyword DomainRegex IPCIDR SrcIPCIDR GeoIP GeoSite ProcessName ProcessPath ProcessNameRegex ProcessPathRegex RuleSet Network DstPort SrcPort InType InName Uid Match") != "OTHER"
		if len(rules) >= 3 && ((i == len(rules)-3 && r.Type == "AND" && r.Payload == tailCorePayload("route-agent-tail-direct") && r.Proxy == "DIRECT") || (i == len(rules)-2 && r.Type == "AND" && r.Payload == tailCorePayload("route-agent-tail-proxy") && r.Proxy == o.c.ProxyGroup)) {
			known = true
		}
		if !known || r.Index != i || r.Extra != nil && r.Extra.Disabled || i < len(rules)-1 && r.Type == "Match" {
			return nil, fmt.Errorf("ambiguous or disabled rule sequence")
		}
	}
	n := len(rules)
	if rules[n-1].Type != "Match" || rules[n-1].Payload != "" || rules[n-1].Proxy == "" {
		return nil, fmt.Errorf("missing unique terminal MATCH")
	}
	if !o.shadow && (rules[n-3].Type != "AND" || rules[n-3].Payload != tailCorePayload("route-agent-tail-direct") || rules[n-3].Proxy != "DIRECT" || rules[n-2].Type != "AND" || rules[n-2].Payload != tailCorePayload("route-agent-tail-proxy") || rules[n-2].Proxy != o.c.ProxyGroup) {
		return nil, fmt.Errorf("core does not have the expected final tail providers and unconditional MATCH")
	}
	return rules, nil
}

func (o *LabObserver) unchanged(ctx context.Context) error {
	if o.control != nil {
		if err := o.control.checkIdentity(); err != nil {
			return err
		}
		if err := o.control.checker.CheckOwner(ctx, o.control.lease); err != nil {
			return err
		}
	}
	rules, err := o.rules(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(o.baseline, rules) {
		return fmt.Errorf("core rule snapshot changed; stop isolated observer")
	}
	return nil
}

func fallbackHost(connection CoreConnection, since time.Time, fallback string) (Domain, bool) {
	if connection.Metadata.DestinationPort != "443" {
		return Domain{}, false
	}
	if connection.ID == "" || !connection.Start.After(since) || connection.Start.After(time.Now().Add(time.Second)) || connection.Rule != "Match" || connection.Payload != "" || len(connection.Chains) == 0 || connection.Chains[len(connection.Chains)-1] != fallback || connection.Metadata.Network != "tcp" || connection.Metadata.SpecialRules != "" || connection.Metadata.SpecialProxy != "" || connection.Metadata.SniffHost != "" {
		return Domain{}, false
	}
	d, err := Normalize(connection.Metadata.Host)
	return d, err == nil && !d.IP && !d.Local
}

func (o *LabObserver) poll(ctx context.Context) error {
	if err := o.unchanged(ctx); err != nil {
		return err
	}
	if o.control != nil {
		if err := o.control.check(); err != nil {
			return err
		}
		if err := o.control.checkPaths(ctx); err != nil {
			return err
		}
	}
	var snapshot struct {
		Connections []CoreConnection `json:"connections"`
	}
	if err := o.get(ctx, "/connections", &snapshot); err != nil {
		return err
	}
	for _, connection := range snapshot.Connections {
		d, ok := fallbackHost(connection, o.started, o.baseline[len(o.baseline)-1].Proxy)
		if !ok || o.attempted[d.Host] {
			continue
		}
		evidence, ok := o.c.LabFixtures[d.Host]
		if o.shadow || o.control != nil {
			ok = o.c.Observation.Allows(d.Host)
		}
		if !ok {
			continue
		}
		if o.attempts.Load() >= uint64(o.c.MaxAPIRequests) {
			break
		}
		o.attempted[d.Host] = true // at most one attempt per allowlisted host/run
		o.attempts.Add(1)
		job, cancel := context.WithTimeout(ctx, time.Duration(o.c.PreflightMS)*time.Millisecond)
		state := State{Hostname: d.Host, Registrable: d.Registrable, Evidence: evidence, Fixture: "synthetic_not_live_network_measurement"}
		var err error
		var decision Decision
		if o.collector != nil {
			_, decision, err = EvaluateEvidence(job, d.Host, o.collector, countedJudge{o})
			if err != nil && o.control != nil {
				cancel()
				return fmt.Errorf("publication blocked during evidence collection: %w", err)
			}
		} else {
			var answer Answer
			answer, err = countedJudge{o}.Decide(job, state)
			decision = Accept(state, answer)
		}
		if o.shadow {
			if err != nil || job.Err() != nil {
				o.failures.Add(1)
			} else if decision != Uncertain {
				if err = o.unchanged(job); err != nil {
					cancel()
					return err
				}
				o.decisions.Add(1)
			}
			cancel()
			continue
		}
		if err == nil && job.Err() == nil && decision != Uncertain {
			// Recheck before mutation. There is no atomic configuration epoch in
			// this API; exclusive ownership is still a lab requirement.
			if err = o.unchanged(job); err != nil {
				o.failures.Add(1)
				cancel()
				return err
			}
			if err = o.saveJournal("reconciling"); err != nil {
				cancel()
				return err
			}
			ttl := time.Duration(o.c.LearnedTTLSeconds) * time.Second
			if o.control != nil && ttl > time.Minute {
				ttl = time.Minute
			}
			err = o.Providers.Change(job, d.Host, &Entry{decision, time.Now().Add(ttl)})
			if err == nil {
				err = o.unchanged(job)
			}
			if err != nil {
				o.failures.Add(1)
				cancel()
				return err
			}
			if err == nil {
				if err = o.saveJournal("active"); err != nil {
					cancel()
					return err
				}
				o.commits.Add(1)
			}
		}
		if err != nil || job.Err() != nil {
			o.failures.Add(1)
		}
		cancel()
	}
	if !o.shadow && o.Providers.Expired() {
		if err := o.unchanged(ctx); err != nil {
			return err
		}
		if err := o.saveJournal("reconciling"); err != nil {
			return err
		}
		if err := o.Providers.Change(ctx, "", nil); err != nil {
			return err
		}
		if err := o.unchanged(ctx); err != nil {
			return err
		}
		return o.saveJournal("active")
	}
	return nil
}

func (o *LabObserver) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	if !o.shadow {
		mux.HandleFunc("/rules/", o.Providers.Handler)
	}
	if o.control != nil {
		mux.HandleFunc("/control/pause", o.handlePause)
	}
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, count, ready := o.Providers.Status()
		if o.shadow {
			ready = o.ready.Load()
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"lab_only": !o.shadow && o.control == nil, "controlled_apply": o.control != nil, "shadow": o.shadow, "attempts": o.attempts.Load(), "shadow_decisions": o.decisions.Load(), "observer_ready": o.ready.Load(), "core_ready": ready, "learned_count": count, "judge_calls": o.judgments.Load(), "commits": o.commits.Load(), "failures": o.failures.Load()})
	})
	return mux
}

func (o *LabObserver) Run(ctx context.Context, statePath string) (runErr error) {
	if !o.shadow && statePath == "" {
		return fmt.Errorf("publishing observer requires a private --state-file")
	}
	if o.control != nil {
		if err := o.control.check(); err != nil {
			return err
		}
		if err := o.control.checker.Check(ctx, netip.MustParseAddr("1.1.1.1")); err != nil {
			return err
		}
		lock, err := os.OpenFile(o.control.path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("publication blocked: ownership_locked; recover explicitly after a crash")
		}
		defer os.Remove(o.control.path + ".lock")
		defer lock.Close()
		if err := json.NewEncoder(lock).Encode(publisherLock{Version: 1, PID: os.Getpid()}); err != nil {
			return err
		}
		if err := lock.Sync(); err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", o.c.HTTPListen)
	if err != nil {
		return err
	}
	o.statePath = statePath
	o.Providers.strictFetch = true
	server := &http.Server{Handler: o.HTTPHandler(), ReadHeaderTimeout: time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 16384}
	if o.control != nil {
		server.WriteTimeout = 25 * time.Second
	}
	defer server.Close()
	// Defer runs before closing HTTP so the owned core can fetch empty bodies.
	// A killed process cannot run this cleanup; the next startup reconciles.
	defer func() {
		if o.shadow {
			o.ready.Store(false)
			return
		}
		if !o.ready.Load() {
			return
		}
		o.ready.Store(false)
		cleanupBudget := 2 * time.Second
		if o.control != nil {
			cleanupBudget = 10 * time.Second
		}
		cleanup, cancel := context.WithTimeout(context.Background(), cleanupBudget)
		defer cancel()
		err := o.unchanged(cleanup)
		if err == nil {
			err = o.reconcile(cleanup)
		}
		if err == nil {
			err = o.saveJournal("stopped")
		} else {
			_ = o.saveJournal("recovery_required")
		}
		if err != nil && runErr == nil {
			runErr = fmt.Errorf("observer exit cleanup incomplete; recovery required")
		}
	}()
	errors := make(chan error, 1)
	go func() { errors <- server.Serve(listener) }()
	// Core can fetch the initial empty providers during this bounded bootstrap.
	startup := time.Now().Add(10 * time.Second)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errors:
			return err
		case <-tick.C:
		}
		budget := time.Second
		if o.shadow || o.control != nil {
			budget = 10 * time.Second
		}
		job, cancel := context.WithTimeout(ctx, budget)
		if o.control != nil {
			select {
			case ack := <-o.control.pause:
				err := o.pauseControlled(job)
				ack <- err
				cancel()
				if err != nil {
					return err
				}
				continue
			default:
			}
			if o.control.paused {
				cancel()
				continue
			}
		}
		if !o.ready.Load() {
			o.baseline, err = o.rules(job)
			if err == nil && o.control != nil && rulesDigest(o.baseline) != o.control.lease.RulesSHA256 {
				cancel()
				return fmt.Errorf("publication blocked: core_snapshot_does_not_match_ownership")
			}
			if err == nil && !o.shadow {
				if err = o.checkJournal(); err != nil {
					cancel()
					return err
				}
				err = o.reconcile(job)
			}
			if err == nil {
				o.started = time.Now()
				o.ready.Store(true)
			}
			cancel()
			if err != nil && time.Now().After(startup) {
				return err
			}
			continue
		}
		err = o.poll(job)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
