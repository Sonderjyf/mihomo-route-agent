package route

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const currentDecisionPolicy = "route-policy-v1"
const maxDecisionRecords = 4096

// Private append-only evidence. It never restores permission or rules on restart.
type DecisionRecord struct {
	ID            string               `json:"id"`
	ConnectionID  string               `json:"connection_id"`
	Host          string               `json:"hostname"`
	TargetIP      string               `json:"target_ip"`
	Started       time.Time            `json:"started_at"`
	Completed     time.Time            `json:"completed_at"`
	Expires       time.Time            `json:"expires_at"`
	Policy        string               `json:"policy_version"`
	Scope         string               `json:"scope"`
	Diagnostic    EvaluationDiagnostic `json:"evaluation"`
	Adopted       bool                 `json:"policy_adopted"`
	Status        string               `json:"status"`
	PublicationID string               `json:"publication_id,omitempty"`
}

type decisionPublisher struct {
	firstDueMS atomic.Int64
	mu         sync.RWMutex
	records    []DecisionRecord
	session    string
	next       time.Time
	interval   time.Duration
}

func randomDecisionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("decision identifier unavailable")
	}
	return hex.EncodeToString(b[:]), nil
}

func newDecisionPublisher(c Config) (*decisionPublisher, error) {
	seconds := c.PublishIntervalSeconds
	if seconds == 0 {
		seconds = 30
	} // Also applies to programmatic constructors.
	if seconds < 1 || seconds > 3600 {
		return nil, fmt.Errorf("publish_interval_seconds must be 1..3600")
	}
	session, err := randomDecisionID()
	return &decisionPublisher{session: session, interval: time.Duration(seconds) * time.Second}, err
}

func (o *LabObserver) decisionScope() string {
	if o.control == nil {
		return ""
	}
	// Expiry may renew; immutable generation fields must not change.
	lease := o.control.lease
	lease.Expires = time.Time{}
	body, _ := json.Marshal(struct {
		Config Config
		Lease  Ownership
		Rules  []CoreRule
		Policy string
	}{o.c, lease, o.baseline, currentDecisionPolicy})
	return digest(body)
}

// Never log this private file or upload it. A new process starts with no records.
// A bounded ledger is intentionally not a database or a cross-session cache.
func (o *LabObserver) appendDecision(record DecisionRecord) error {
	if o.statePath == "" {
		return nil
	} // Unit tests without a lifecycle journal.
	path := o.statePath + ".decisions.jsonl"
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Size() >= 16<<20 {
			return fmt.Errorf("decision ledger limit or type invalid")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("decision ledger unavailable")
	}
	body, err := json.Marshal(struct {
		Version int            `json:"version"`
		Session string         `json:"session"`
		Record  DecisionRecord `json:"record"`
	}{1, o.publisher.session, record})
	if err != nil {
		return fmt.Errorf("decision encoding failed")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("decision ledger unavailable")
	}
	body = append(body, '\n')
	n, writeErr := f.Write(body)
	if writeErr == nil && n != len(body) {
		writeErr = fmt.Errorf("short decision write")
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return fmt.Errorf("decision ledger write failed")
	}
	return nil
}

func (o *LabObserver) recordDecision(ctx context.Context, d Domain, r EvaluationDiagnostic, connectionID, targetIP, scope string, started time.Time) error {
	p := o.publisher
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.records) >= maxDecisionRecords {
		return fmt.Errorf("decision record capacity reached")
	}
	id, err := randomDecisionID()
	if err != nil {
		return err
	}
	ttl := time.Duration(o.c.LearnedTTLSeconds) * time.Second
	if ttl > time.Minute {
		ttl = time.Minute
	}
	rec := DecisionRecord{ID: id, ConnectionID: connectionID, Host: d.Host, TargetIP: targetIP,
		Started: started, Completed: time.Now(), Expires: started.Add(ttl), Policy: currentDecisionPolicy,
		Scope: scope, Diagnostic: r, Status: "rejected"}
	rec.Adopted = ctx.Err() == nil && r.StageError == "none" && r.PolicyReason == "accepted" && (r.Decision == Direct || r.Decision == Proxy)
	if rec.Adopted && time.Now().Before(rec.Expires) {
		rec.Status = "pending"
		// A second suggestion cannot extend an existing result without new publication.
		for _, old := range p.records {
			if old.Host == rec.Host && (old.Status == "pending" || old.Status == "applied") && time.Now().Before(old.Expires) && old.Scope == rec.Scope {
				rec.Status = "superseded"
				break
			}
		}
	}
	if err := o.appendDecision(rec); err != nil {
		return err
	}
	p.records = append(p.records, rec)
	return nil
}

func (o *LabObserver) decisionReceipts() []map[string]any {
	if o.publisher == nil {
		return nil
	}
	p := o.publisher
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]map[string]any, 0, len(p.records))
	for _, r := range p.records {
		out = append(out, map[string]any{"id": r.ID, "connection_id": r.ConnectionID,
			"choice": r.Diagnostic.Choice, "decision": r.Diagnostic.Decision,
			"policy_reason": r.Diagnostic.PolicyReason, "policy_adopted": r.Adopted,
			"status": r.Status, "expires_at": r.Expires, "policy_version": r.Policy,
			"scope": r.Scope, "publication_id": r.PublicationID})
	}
	return out
}

func (o *LabObserver) endDecisions(status string, expiredOnly bool) error {
	if o.publisher == nil {
		return nil
	}
	p := o.publisher
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, r := range p.records {
		if (r.Status != "pending" && r.Status != "applied") || (expiredOnly && time.Now().Before(r.Expires)) {
			continue
		}
		r.Status = status
		if err := o.appendDecision(r); err != nil {
			return err
		}
		p.records[i] = r
	}
	return nil
}

// Called by the serial worker on its own tick context, never the judgment job.
// Initialization occurs only after empty bootstrap and current ownership checks.
func (o *LabObserver) publishDue(ctx context.Context) error {
	p := o.publisher
	if p == nil || p.next.IsZero() || time.Now().Before(p.next) {
		return nil
	}
	now := time.Now()
	p.next = p.next.Add((now.Sub(p.next)/p.interval + 1) * p.interval)
	if err := o.control.check(); err != nil {
		return err
	}
	if err := o.unchanged(ctx); err != nil {
		return err
	}
	if err := o.control.checkPaths(ctx); err != nil {
		return err
	}
	if o.control.paused || o.control.maintenance.due(time.Now()) {
		return nil
	}
	scope := o.decisionScope()
	next := map[string]Entry{}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, r := range p.records {
		if r.Status != "pending" && r.Status != "applied" {
			continue
		}
		status := r.Status
		if !time.Now().Before(r.Expires) {
			status = "expired"
		} else if !r.Adopted || r.Policy != currentDecisionPolicy || r.Scope != scope {
			status = "invalidated"
		}
		if status != r.Status {
			r.Status = status
			if err := o.appendDecision(r); err != nil {
				return err
			}
			p.records[i] = r
			continue
		}
		next[r.Host] = Entry{Decision: r.Diagnostic.Decision, Expires: r.Expires}
	}
	o.Providers.mu.RLock()
	same := len(o.Providers.entries) == len(next) && !o.Providers.dirty
	for host, entry := range next {
		old, ok := o.Providers.entries[host]
		if !ok || old.Decision != entry.Decision || !old.Expires.Equal(entry.Expires) {
			same = false
		}
	}
	o.Providers.mu.RUnlock()
	if same {
		return o.Providers.Check(ctx) // Readback only: detect drift without re-submission.
	}
	if err := o.saveJournal("reconciling"); err != nil {
		return err
	}
	if err := o.Providers.Replace(ctx, next); err != nil {
		for i, r := range p.records {
			if r.Status == "pending" {
				r.Status = "failed"
				if persistErr := o.appendDecision(r); persistErr != nil {
					return persistErr
				}
				p.records[i] = r
			}
		}
		return err
	}
	if err := o.unchanged(ctx); err != nil {
		return err
	}
	if err := o.control.check(); err != nil {
		return err
	}
	if err := o.saveJournal("active"); err != nil {
		return err
	}
	generation, _, _ := o.Providers.Status()
	publicationID := fmt.Sprintf("%s-%d", p.session, generation)
	for i, r := range p.records {
		if r.Status == "pending" {
			r.Status, r.PublicationID = "applied", publicationID
			if err := o.appendDecision(r); err != nil {
				return err
			}
			p.records[i] = r
		}
	}
	o.commits.Add(1)
	return nil
}

func (o *LabObserver) publicationSchedule() map[string]any {
	if o.publisher == nil {
		return nil
	}
	return map[string]any{"interval_seconds": int(o.publisher.interval / time.Second),
		"first_due_unix_ms":                 o.publisher.firstDueMS.Load(),
		"first_nonempty_submission_unix_ms": o.Providers.firstNonemptyMS.Load(),
		"nonempty_submissions":              o.Providers.nonemptySubmissions.Load()}
}
