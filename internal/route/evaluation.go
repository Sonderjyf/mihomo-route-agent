package route

import (
	"context"
	"errors"
	"net/netip"
	"time"
)

// EvaluationDiagnostic is a bounded, host-free summary, not a history of model
// responses. A valid choice is a suggestion; only Decision is the policy result.
type EvaluationDiagnostic struct {
	Version                int      `json:"version"`
	Stage                  string   `json:"stage"`
	StageError             string   `json:"stage_error"`
	PolicyReason           string   `json:"policy_reason"`
	Choice                 Decision `json:"choice"`
	Decision               Decision `json:"decision"`
	Evidence               Evidence `json:"evidence"`
	DNS                    string   `json:"dns"`
	DeadlineMS             int64    `json:"deadline_ms"`
	ElapsedMS              int64    `json:"elapsed_ms"`
	DNSMS                  int64    `json:"dns_ms"`
	PathMS                 int64    `json:"path_ms"`
	DirectTLSMS            int64    `json:"direct_tls_ms"`
	ProxyTLSMS             int64    `json:"proxy_tls_ms"`
	CollectionMS           int64    `json:"collection_ms"`
	ModelMS                int64    `json:"model_ms"`
	PublicationMS          int64    `json:"publication_ms"`
	JobDeadlineMS          int64    `json:"job_deadline_ms"`
	JobElapsedMS           int64    `json:"job_elapsed_ms"`
	ModelCalls             int      `json:"model_calls"`
	TransportAttempts      uint64   `json:"transport_attempts"`
	TransportAttemptsKnown bool     `json:"transport_attempts_known"`
	Publication            string   `json:"publication"`
	ConnectionTargetMatch  bool     `json:"connection_target_match"`
}

type EvaluationResult struct {
	State      State
	Diagnostic EvaluationDiagnostic
	// Internal correlation only. Never add the target address to public output.
	target netip.Addr
}

func boundedMS(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	if ms > 600000 {
		return 600000
	}
	return ms
}

func remainingMS(ctx context.Context) int64 {
	if end, ok := ctx.Deadline(); ok {
		return boundedMS(time.Until(end))
	}
	return 10000
}

func stageError(ctx context.Context, fallback string) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if ctx.Err() != nil {
		return "canceled"
	}
	return fallback
}

type transportCounter interface{ transportAttempts() (uint64, bool) }

func modelAttempts(j Judge) (uint64, bool) {
	if counter, ok := j.(transportCounter); ok {
		return counter.transportAttempts()
	}
	return 0, false
}

// EvaluateEvidenceDetailed retains the existing ten-second upper bound and all
// Accept gates. Collection/model errors and policy refusals are separate labels.
func EvaluateEvidenceDetailed(parent context.Context, host string, collector EvidenceCollector, judge Judge) (result EvaluationResult, resultErr error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	r := &result.Diagnostic
	*r = EvaluationDiagnostic{Version: 1, Stage: "normalize", StageError: "none", PolicyReason: "not_evaluated", Choice: Uncertain, Decision: Uncertain, Evidence: Evidence{"not_tested", "not_tested"}, DNS: "not_tested", DeadlineMS: remainingMS(ctx), Publication: "not_attempted"}
	defer func() { r.ElapsedMS = boundedMS(time.Since(start)) }()
	d, err := Normalize(host)
	if err != nil {
		r.StageError = "input_invalid"
		return result, err
	}
	result.State = State{Hostname: d.Host, Registrable: d.Registrable, Evidence: r.Evidence}
	if collector == nil || judge == nil {
		r.StageError = "input_invalid"
		return result, errAcceptance
	}
	_, r.TransportAttemptsKnown = modelAttempts(judge)
	r.Stage = "collection"
	started := time.Now()
	probe, err := collector.Collect(ctx, d)
	r.CollectionMS = boundedMS(time.Since(started))
	r.DNSMS, r.PathMS, r.DirectTLSMS, r.ProxyTLSMS = probe.timing.DNSMS, probe.timing.PathMS, probe.timing.DirectTLSMS, probe.timing.ProxyTLSMS
	r.DNS, r.Evidence, result.State.Evidence, result.target = probe.DNS, probe.Evidence, probe.Evidence, probe.target
	if probe.stage != "" {
		r.Stage = probe.stage
	}
	if err != nil {
		r.StageError = stageError(ctx, "collection_failed")
		return result, err
	}
	if ctx.Err() != nil {
		r.StageError = stageError(ctx, "collection_failed")
		return result, ctx.Err()
	}
	if r.Evidence.DirectTLS != "verified_success" && !(r.Evidence.DirectTLS == "repeated_failure" && r.Evidence.ProxyTLS == "verified_success") {
		r.Stage, r.PolicyReason = "policy", "evidence_insufficient"
		return result, nil
	}
	r.Stage, r.ModelCalls = "model", 1
	before, known := modelAttempts(judge)
	started = time.Now()
	answer, err := judge.Decide(ctx, result.State)
	r.ModelMS = boundedMS(time.Since(started))
	after, knownAfter := modelAttempts(judge)
	r.TransportAttemptsKnown = known && knownAfter && after >= before
	if r.TransportAttemptsKnown {
		r.TransportAttempts = after - before
	}
	if err != nil {
		r.StageError = stageError(ctx, acceptanceModelReason(err))
		return result, err
	}
	if answer.Choice.Valid() {
		r.Choice = answer.Choice
	}
	if ctx.Err() != nil {
		r.StageError = stageError(ctx, "model_unavailable")
		return result, ctx.Err()
	}
	r.Stage = "policy"
	r.Decision, r.PolicyReason = acceptWithReason(result.State, answer)
	return result, nil
}
