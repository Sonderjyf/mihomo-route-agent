package route

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

type negativeEntry struct {
	match   Match
	expires time.Time
}

type pendingDecision struct {
	ctx     context.Context
	cancel  context.CancelFunc
	waiters int
}

type Agent struct {
	Config     Config
	Matcher    *Matcher
	Providers  *Providers
	judge      Judge
	root       context.Context
	flight     singleflight.Group
	pendingMu  sync.Mutex
	pending    map[string]*pendingDecision
	slots      chan struct{}
	workers    chan struct{}
	negativeMu sync.Mutex
	negative   map[string]negativeEntry
	queries    atomic.Uint64
	judgments  atomic.Uint64
	commits    atomic.Uint64
	failures   atomic.Uint64
	inflight   atomic.Int64
}

func New(ctx context.Context, c Config, judge Judge, secret string) (*Agent, error) {
	m, err := NewMatcher(c.Rules)
	if err != nil {
		return nil, err
	}
	p, err := NewProviders(c.Controller, secret, c.Capacity)
	if err != nil {
		return nil, err
	}
	return &Agent{Config: c, Matcher: m, Providers: p, judge: judge, root: ctx, slots: make(chan struct{}, 4), workers: make(chan struct{}, 2), negative: map[string]negativeEntry{}, pending: map[string]*pendingDecision{}}, nil
}

func hostID(host string) string {
	digest := sha256.Sum256([]byte(host))
	return hex.EncodeToString(digest[:6])
}

func (a *Agent) negativeMatch(host string) (Match, bool) {
	a.negativeMu.Lock()
	defer a.negativeMu.Unlock()
	item, ok := a.negative[host]
	return item.match, ok && time.Now().Before(item.expires)
}

func (a *Agent) remember(host string, match Match) {
	a.negativeMu.Lock()
	defer a.negativeMu.Unlock()
	for key, item := range a.negative {
		if !time.Now().Before(item.expires) {
			delete(a.negative, key)
		}
	}
	if len(a.negative) < a.Config.Capacity {
		a.negative[host] = negativeEntry{match, time.Now().Add(30 * time.Second)}
	}
}

func (a *Agent) fallback(source string) Match { return Match{Uncertain, source} }

func (a *Agent) join(host string) *pendingDecision {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	ticket, ok := a.pending[host]
	if !ok {
		ctx, cancel := context.WithTimeout(a.root, time.Duration(a.Config.PreflightMS)*time.Millisecond)
		ticket = &pendingDecision{ctx: ctx, cancel: cancel}
		a.pending[host] = ticket
	}
	if a.Config.Mode != "async" {
		ticket.waiters++
	}
	return ticket
}

func (a *Agent) leave(host string, ticket *pendingDecision) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	ticket.waiters--
	if ticket.waiters == 0 {
		ticket.cancel()
		if a.pending[host] == ticket {
			delete(a.pending, host)
			a.flight.Forget(host)
		}
	}
}

func (a *Agent) finished(host string, ticket *pendingDecision) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if a.pending[host] == ticket {
		delete(a.pending, host)
	}
	if a.Config.Mode == "async" {
		ticket.cancel()
	}
}

// Resolve shares a bounded job across QTYPEs. Caller cancellation only stops
// that caller's wait; the shared job retains its own absolute deadline.
func (a *Agent) Resolve(ctx context.Context, d Domain) Match {
	if ctx.Err() != nil {
		return a.fallback("query-canceled")
	}
	match := a.Matcher.Match(d)
	if match.Source != "unknown" {
		return match
	}
	if entry, ok := a.Providers.Lookup(d.Host); ok {
		return Match{entry.Decision, "learned"}
	}
	if match, ok := a.negativeMatch(d.Host); ok {
		return match
	}
	if a.Config.Mode == "off" || a.judge == nil {
		return a.fallback("disabled")
	}
	_, _, ready := a.Providers.Status()
	if a.Config.Controller == "" || !ready {
		return a.fallback("core-not-ready")
	}
	ticket := a.join(d.Host)
	if a.Config.Mode != "async" {
		defer a.leave(d.Host, ticket)
	}
	result := a.flight.DoChan(d.Host, func() (any, error) {
		defer a.finished(d.Host, ticket)
		if entry, ok := a.Providers.Lookup(d.Host); ok {
			return Match{entry.Decision, "learned"}, nil
		}
		if match, ok := a.negativeMatch(d.Host); ok {
			return match, nil
		}
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
		default:
			return a.fallback("queue-full"), nil
		}
		job := ticket.ctx
		a.inflight.Add(1)
		defer a.inflight.Add(-1)
		select {
		case a.workers <- struct{}{}:
			defer func() { <-a.workers }()
		case <-job.Done():
			return a.fallback("queue-timeout"), nil
		}
		// Check A and AAAA together even if the first request was HTTPS/SVCB.
		if protected, err := a.guardHost(job, d.Host); err != nil {
			return a.finish(d.Host, a.fallback("dns-evidence-unavailable")), nil
		} else if protected {
			return Match{Direct, "hard-dns"}, nil
		}
		if a.judgments.Load() >= uint64(a.Config.MaxAPIRequests) {
			return a.finish(d.Host, a.fallback("request-budget")), nil
		}
		// Two workers may reach the budget together; reserve a call under a lock.
		a.negativeMu.Lock()
		if a.judgments.Load() >= uint64(a.Config.MaxAPIRequests) {
			a.negativeMu.Unlock()
			return a.finish(d.Host, a.fallback("request-budget")), nil
		}
		a.judgments.Add(1)
		a.negativeMu.Unlock()
		state := State{Hostname: d.Host, Registrable: d.Registrable, Evidence: Evidence{"not_tested", "not_tested"}}
		if fixture, ok := a.Config.LabFixtures[d.Host]; ok {
			state.Evidence = fixture
			state.Fixture = "synthetic_not_live_network_measurement"
		}
		slog.Info("judge_started", "host_id", hostID(d.Host))
		answer, err := a.judge.Decide(job, state)
		if err != nil || job.Err() != nil {
			a.failures.Add(1)
			return a.finish(d.Host, a.fallback("judge-failed-or-timeout")), nil
		}
		decision := Accept(state, answer)
		slog.Info("judge_completed", "host_id", hostID(d.Host), "raw", answer.Choice, "accepted", decision)
		if decision == Uncertain {
			return a.finish(d.Host, a.fallback("uncertain")), nil
		}
		entry := Entry{decision, time.Now().Add(time.Duration(a.Config.LearnedTTLSeconds) * time.Second)}
		if err := a.Providers.Change(job, d.Host, &entry); err != nil {
			a.failures.Add(1)
			return a.finish(d.Host, a.fallback("commit-failed")), nil
		}
		a.commits.Add(1)
		slog.Info("provider_applied", "host_id", hostID(d.Host), "route", decision)
		return Match{decision, "model-applied"}, nil
	})
	if a.Config.Mode == "async" {
		return a.fallback("async-pending")
	}
	select {
	case value := <-result:
		if value.Err != nil {
			return a.fallback("decision-error")
		}
		return value.Val.(Match)
	case <-ctx.Done():
		return a.fallback("query-timeout")
	case <-ticket.ctx.Done():
		return a.fallback("preflight-timeout")
	}
}

func (a *Agent) finish(host string, match Match) Match { a.remember(host, match); return match }

func (a *Agent) maintain(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.Config.Controller != "" {
				check, cancel := context.WithTimeout(ctx, time.Second)
				_ = a.Providers.Check(check)
				cancel()
			}
			_, _, ready := a.Providers.Status()
			if a.Config.Controller != "" && (!ready || a.Providers.Expired()) {
				job, cancel := context.WithTimeout(ctx, time.Second)
				_ = a.Providers.Change(job, "", nil)
				cancel()
			}
		}
	}
}
