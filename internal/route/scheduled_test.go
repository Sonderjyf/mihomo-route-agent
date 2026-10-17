package route

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func preparedScheduledFixture(t *testing.T) *LabObserver {
	t.Helper()
	o, _, _ := controlledFixture(t)
	o.Providers.mu.Lock()
	o.Providers.dirty = false // Run normally establishes this with empty bootstrap ACK.
	o.Providers.mu.Unlock()
	o.publisher.next = time.Now().Add(time.Hour)
	return o
}

func TestScheduledJudgmentCannotPublishBeforeTickAndUsesFreshContext(t *testing.T) {
	o := preparedScheduledFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	if err := o.poll(ctx); err != nil {
		t.Fatal(err)
	}
	cancel() // The completed judgment grants no ongoing permission.
	if o.commits.Load() != 0 || len(o.decisionReceipts()) != 1 || o.decisionReceipts()[0]["status"] != "pending" {
		t.Fatal("not recorded pending")
	}
	if err := o.publishDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, count, _ := o.Providers.Status(); count != 0 {
		t.Fatal("published before tick")
	}
	if o.EvaluationDiagnostic().Publication != "not_attempted" {
		t.Fatal("evaluation claimed publication")
	}
	o.publisher.next = time.Now().Add(-time.Millisecond)
	if err := o.publishDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	receipts := o.decisionReceipts()
	if o.commits.Load() != 1 || receipts[0]["status"] != "applied" || receipts[0]["publication_id"] == "" {
		t.Fatal("missing applied receipt")
	}
	gen, _, _ := o.Providers.Status()
	o.publisher.next = time.Now().Add(-time.Millisecond)
	if err := o.publishDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _, _ := o.Providers.Status()
	if gen != after || o.commits.Load() != 1 {
		t.Fatal("unchanged payload republished")
	}
	if err := o.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if o.decisionReceipts()[0]["status"] != "withdrawn" {
		t.Fatal("drain retained authority")
	}
}

func TestScheduledExpiryRejectionAndVersionChangesDoNotBecomeRules(t *testing.T) {
	for _, change := range []string{"expiry", "policy", "rules", "uncertain", "late"} {
		t.Run(change, func(t *testing.T) {
			o := preparedScheduledFixture(t)
			if change == "uncertain" {
				o.judge = Stub{Answer: validAnswer(Uncertain)}
			}
			if change == "late" {
				o.c.PreflightMS = 5
				o.judge = Stub{Answer: validAnswer(Proxy), Wait: func(ctx context.Context) error { <-ctx.Done(); return nil }}
			}
			err := o.poll(context.Background())
			if change != "late" && err != nil {
				t.Fatal(err)
			}
			if change == "late" && err == nil {
				t.Fatal("late result accepted")
			}
			switch change {
			case "expiry":
				o.publisher.records[0].Expires = time.Now().Add(-time.Second)
			case "policy":
				o.publisher.records[0].Policy = "obsolete-policy"
			case "rules":
				o.c.Rules = []Rule{{Type: "DOMAIN", Value: "example.com", Route: Direct, Source: "manual"}}
			}
			o.publisher.next = time.Now().Add(-time.Millisecond)
			if err := o.publishDue(context.Background()); err != nil {
				t.Fatal(err)
			}
			if o.commits.Load() != 0 {
				t.Fatal("invalid result committed")
			}
			if _, count, _ := o.Providers.Status(); count != 0 {
				t.Fatal("invalid rule remains")
			}
			if o.decisionReceipts()[0]["status"] == "applied" {
				t.Fatal("invalid result applied")
			}
		})
	}
}

func TestScheduledTickRechecksOwnerAndNeverRestoresLedger(t *testing.T) {
	for _, change := range []string{"profile", "core", "lease"} {
		t.Run(change, func(t *testing.T) {
			o, guard, puts := controlledFixture(t)
			if err := o.poll(context.Background()); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "profile":
				if err := os.WriteFile(o.control.lease.ConfigPath, []byte("different profile"), 0600); err != nil {
					t.Fatal(err)
				}
			case "core":
				guard.restarted.Store(true)
			case "lease":
				o.control.lease.Expires = time.Now().Add(-time.Second)
			}
			o.publisher.next = time.Now().Add(-time.Millisecond)
			if err := o.publishDue(context.Background()); err == nil {
				t.Fatal("stale ownership accepted")
			}
			if puts.Load() != 0 || o.commits.Load() != 0 {
				t.Fatal("write under stale authority")
			}
		})
	}
	o := preparedScheduledFixture(t)
	if err := o.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := o.publisher.session
	fresh, err := newDecisionPublisher(o.c)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.session == before || len(fresh.records) != 0 || !fresh.next.IsZero() {
		t.Fatal("restored pending authority")
	}
	raw, err := os.ReadFile(o.statePath + ".decisions.jsonl")
	if err != nil || !strings.Contains(string(raw), o.publisher.records[0].ID) {
		t.Fatal("missing durable correlation")
	}
	for _, receipt := range o.decisionReceipts() {
		if _, ok := receipt["hostname"]; ok {
			t.Fatal("private hostname exposed")
		}
		if _, ok := receipt["target_ip"]; ok {
			t.Fatal("private address exposed")
		}
	}
}

func TestScheduledFailedACKCannotBeApplied(t *testing.T) {
	o := preparedScheduledFixture(t)
	if err := o.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	base := o.Providers.client.Transport
	o.Providers.client.Transport = localModelTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPut {
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		}
		return base.RoundTrip(r)
	})
	o.publisher.next = time.Now().Add(-time.Millisecond)
	if err := o.publishDue(context.Background()); err == nil {
		t.Fatal("failed ACK accepted")
	}
	if o.commits.Load() != 0 || o.decisionReceipts()[0]["status"] != "failed" {
		t.Fatal("failed publication marked applied")
	}
}

func TestScheduledExpiryDuringACKCannotBeApplied(t *testing.T) {
	o := preparedScheduledFixture(t)
	if err := o.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(500 * time.Millisecond)
	lateACKObserved := false
	o.publisher.records[0].Expires = expires
	base := o.Providers.client.Transport
	o.Providers.client.Transport = localModelTransport(func(r *http.Request) (*http.Response, error) {
		res, err := base.RoundTrip(r)
		if err == nil && r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "proxy") {
			// Both writes were valid when submitted; only the last ACK is late.
			lateACKObserved = time.Now().Before(expires)
			time.Sleep(time.Until(expires) + time.Millisecond)
		}
		return res, err
	})
	o.publisher.next = time.Now().Add(-time.Millisecond)
	if err := o.publishDue(context.Background()); err == nil {
		t.Fatal("expired ACK accepted")
	}
	if !lateACKObserved {
		t.Fatal("test did not reach the valid submission / expired ACK boundary")
	}
	if o.commits.Load() != 0 || o.decisionReceipts()[0]["status"] == "applied" {
		t.Fatal("expired ACK marked applied")
	}
	if err := o.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, count, _ := o.Providers.Status(); count != 0 {
		t.Fatal("expired cache not drained")
	}
}
