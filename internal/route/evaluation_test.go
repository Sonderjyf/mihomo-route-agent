package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type diagnosticCollector struct {
	evidence Evidence
	err      error
}

func (c diagnosticCollector) Collect(context.Context, Domain) (ProbeReport, error) {
	return ProbeReport{Evidence: c.evidence, DNS: "resolved"}, c.err
}

func TestEvaluationReasonsContractAndCompatibility(t *testing.T) {
	const canary = "SECRET_CANARY_NEVER_EMIT"
	for _, name := range []string{"accepted", "threshold", "evidence", "null", "model_error", "collection_error", "late"} {
		t.Run(name, func(t *testing.T) {
			collector := diagnosticCollector{evidence: Evidence{"verified_success", "not_tested"}}
			judge := Judge(Stub{Answer: validAnswer(Direct)})
			wantReason, wantError, wantDecision := "accepted", "none", Direct
			switch name {
			case "threshold":
				a := validAnswer(Direct)
				a.Probabilities = map[Decision]float64{Direct: .7, Proxy: .2, Uncertain: .1}
				judge, wantReason, wantDecision = Stub{Answer: a}, "probability_below_threshold", Uncertain
			case "evidence":
				collector.evidence.DirectTLS = "not_tested"
				wantReason, wantDecision = "evidence_insufficient", Uncertain
			case "null":
				judge = &Jev{key: canary, client: &http.Client{Transport: localModelTransport(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"answers":{"route":{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":1,"PROXY":null,"UNCERTAIN":0}}}}`))}, nil
				})}}
				wantReason, wantError, wantDecision = "not_evaluated", "model_response_invalid", Uncertain
			case "model_error":
				judge = Stub{Wait: func(context.Context) error { return fmt.Errorf(canary) }}
				wantReason, wantError, wantDecision = "not_evaluated", "model_unavailable", Uncertain
			case "collection_error":
				collector.err = fmt.Errorf(canary)
				wantReason, wantError, wantDecision = "not_evaluated", "collection_failed", Uncertain
			case "late":
				judge = Stub{Answer: validAnswer(Direct), Wait: func(ctx context.Context) error { <-ctx.Done(); return nil }}
				wantReason, wantError, wantDecision = "not_evaluated", "deadline_exceeded", Uncertain
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			r, err := EvaluateEvidenceDetailed(ctx, "example.com", collector, judge)
			if r.Diagnostic.Decision != wantDecision || r.Diagnostic.PolicyReason != wantReason || r.Diagnostic.StageError != wantError || (err != nil) != (wantError != "none") {
				t.Fatalf("%+v error=%v", r.Diagnostic, err)
			}
			if r.Diagnostic.DeadlineMS > 40 || r.Diagnostic.ElapsedMS > 1000 {
				t.Fatal("budget not inherited")
			}
			body, _ := json.Marshal(r.Diagnostic)
			for _, forbidden := range []string{canary, "example.com", "probabilities", "confidence", "hostname"} {
				if strings.Contains(string(body), forbidden) {
					t.Fatal("diagnostic leaked private data")
				}
			}
			if name == "evidence" && r.Diagnostic.ModelCalls != 0 {
				t.Fatal("unqualified evidence called model")
			}
			if name != "late" {
				_, decision, oldErr := EvaluateEvidence(context.Background(), "example.com", collector, judge)
				if decision != wantDecision || (oldErr != nil) != (err != nil) {
					t.Fatal("compatibility wrapper changed decision")
				}
			}
		})
	}
	j, err := NewJev("synthetic-key", "")
	if err != nil || j.transport.base.ForceAttemptHTTP2 || !j.transport.base.DisableKeepAlives || j.transport.base.Protocols.HTTP2() || !j.transport.base.Protocols.HTTP1() {
		t.Fatal("production transport can hide replays")
	}
	if n, known := j.transportAttempts(); n != 0 || !known {
		t.Fatal("unknown counter before transport")
	}
}

func TestControlledDiagnosticRejectsLateModelWithoutPublication(t *testing.T) {
	o, _, puts := controlledFixture(t)
	o.c.PreflightMS = 40
	o.collector = diagnosticCollector{evidence: Evidence{"verified_success", "not_tested"}}
	o.judge = Stub{Answer: validAnswer(Direct), Wait: func(ctx context.Context) error { <-ctx.Done(); return nil }}
	if err := o.poll(context.Background()); err == nil {
		t.Fatal("late result accepted")
	}
	if puts.Load() != 0 || o.commits.Load() != 0 {
		t.Fatal("late nonempty publication")
	}
	r := o.lastEvaluation.Load().(EvaluationDiagnostic)
	if r.Stage != "model" || r.StageError != "deadline_exceeded" || r.Decision != Uncertain || r.Publication != "not_attempted" || r.JobDeadlineMS > 40 {
		t.Fatalf("%+v", r)
	}
}
