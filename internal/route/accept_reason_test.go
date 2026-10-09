package route

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestAcceptancePolicyReasons(t *testing.T) {
	direct := Evidence{"verified_success", "not_tested"}
	proxy := Evidence{"repeated_failure", "verified_success"}
	for _, tc := range []struct {
		name     string
		evidence Evidence
		edit     func(*Answer)
		want     Decision
		reason   string
	}{
		{"direct", direct, nil, Direct, "accepted"},
		{"proxy", proxy, func(a *Answer) { *a = validAnswer(Proxy) }, Proxy, "accepted"},
		{"type", direct, func(a *Answer) { a.Type = "SYNTHETIC_CANARY" }, Uncertain, "answer_type_invalid"},
		{"choice", direct, func(a *Answer) { a.Choice = "SYNTHETIC_CANARY" }, Uncertain, "choice_invalid"},
		{"missing_map", direct, func(a *Answer) { a.Probabilities = nil }, Uncertain, "probability_count_invalid"},
		{"extra_label", direct, func(a *Answer) { a.Probabilities["SYNTHETIC_CANARY"] = 0 }, Uncertain, "probability_count_invalid"},
		{"replaced_label", direct, func(a *Answer) { delete(a.Probabilities, Proxy); a.Probabilities["SYNTHETIC_CANARY"] = 0 }, Uncertain, "probability_value_invalid"},
		{"negative", direct, func(a *Answer) { a.Probabilities[Proxy] = -.1 }, Uncertain, "probability_value_invalid"},
		{"above_one", direct, func(a *Answer) { a.Probabilities[Direct] = 1.1 }, Uncertain, "probability_value_invalid"},
		{"nan", direct, func(a *Answer) { a.Probabilities[Direct] = math.NaN() }, Uncertain, "probability_value_invalid"},
		{"infinity", direct, func(a *Answer) { a.Probabilities[Direct] = math.Inf(1) }, Uncertain, "probability_value_invalid"},
		{"sum", direct, func(a *Answer) { a.Probabilities[Direct] = .6 }, Uncertain, "probability_sum_invalid"},
		{"sum_tolerance", direct, func(a *Answer) { a.Probabilities[Direct] = .99 }, Direct, "accepted"},
		{"below_threshold_high_confidence", direct, func(a *Answer) { a.Probabilities[Direct] = .79; a.Probabilities[Proxy] = .21; a.Confidence = 1 }, Uncertain, "probability_below_threshold"},
		{"threshold_low_confidence", direct, func(a *Answer) { a.Probabilities[Direct] = .8; a.Probabilities[Proxy] = .2; a.Confidence = 0 }, Direct, "accepted"},
		{"unknown_evidence", Evidence{"not_tested", "not_tested"}, nil, Uncertain, "evidence_insufficient"},
		{"proxy_without_direct_failure", Evidence{"not_tested", "verified_success"}, func(a *Answer) { *a = validAnswer(Proxy) }, Uncertain, "evidence_insufficient"},
		{"direct_without_proxy_success", Evidence{"repeated_failure", "not_tested"}, func(a *Answer) { *a = validAnswer(Proxy) }, Uncertain, "evidence_insufficient"},
		{"contrary_choice", direct, func(a *Answer) { *a = validAnswer(Proxy) }, Uncertain, "choice_evidence_mismatch"},
		{"model_uncertain", direct, func(a *Answer) { *a = validAnswer(Uncertain) }, Uncertain, "choice_evidence_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := validAnswer(Direct)
			if tc.edit != nil {
				tc.edit(&a)
			}
			s := State{Hostname: "SYNTHETIC_CANARY", Evidence: tc.evidence}
			got, reason := acceptWithReason(s, a)
			if got != tc.want || reason != tc.reason || Accept(s, a) != tc.want {
				t.Fatalf("decision=%s reason=%s", got, reason)
			}
			if strings.Contains(reason, "SYNTHETIC_CANARY") {
				t.Fatal("raw answer leaked")
			}
		})
	}
}

func TestJevWirePolicyAndNullProbabilities(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
		want               Decision
		parseError         bool
	}{
		{"documented_shape", `{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":0.84,"PROXY":0.16,"UNCERTAIN":0},"confidence":0.75}`, "accepted", Direct, false},
		{"below_threshold", `{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":0.79,"PROXY":0.2,"UNCERTAIN":0.01},"confidence":1}`, "probability_below_threshold", Uncertain, false},
		{"missing_type", `{"choice":"DIRECT","probabilities":{"DIRECT":1,"PROXY":0,"UNCERTAIN":0}}`, "answer_type_invalid", Uncertain, false},
		{"missing_distribution", `{"type":"choice","choice":"DIRECT","confidence":1}`, "probability_count_invalid", Uncertain, false},
		{"null_distribution", `{"type":"choice","choice":"DIRECT","probabilities":null}`, "probability_count_invalid", Uncertain, false},
		{"null_value", `{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":0.9,"PROXY":0.1,"UNCERTAIN":null}}`, "", Uncertain, true},
		{"string_value", `{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":"SYNTHETIC_CANARY","PROXY":0,"UNCERTAIN":0}}`, "", Uncertain, true},
		{"wrong_keys", `{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":1,"PROXY":0,"SYNTHETIC_CANARY":0}}`, "probability_value_invalid", Uncertain, false},
		{"sum", `{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":0.6,"PROXY":0,"UNCERTAIN":0}}`, "probability_sum_invalid", Uncertain, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := State{Hostname: "example.com", Evidence: Evidence{"verified_success", "not_tested"}}
			calls := 0
			j := &Jev{key: "synthetic-only", client: &http.Client{Transport: localModelTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var request struct {
					State State `json:"state"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil || request.State != state {
					t.Fatal("model evidence changed")
				}
				// Literal external wire shape, not Marshal(Answer); no network.
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"answers":{"route":` + tc.body + `}}`)), Header: make(http.Header)}, nil
			})}}
			a, err := j.Decide(context.Background(), state)
			if calls != 1 || (err != nil) != tc.parseError {
				t.Fatal("unexpected parser outcome")
			}
			if err != nil {
				if strings.Contains(err.Error(), "SYNTHETIC_CANARY") || acceptanceModelReason(err) != "model_response_invalid" {
					t.Fatal("unsafe parse diagnostic")
				}
				return
			}
			got, reason := acceptWithReason(state, a)
			if got != tc.want || reason != tc.reason {
				t.Fatalf("decision=%s reason=%s", got, reason)
			}
		})
	}
}

func TestLegacyNullCoercionReproduction(t *testing.T) {
	// Demonstrates the old direct-to-float64 decoder's bug with synthetic data.
	var old Answer
	if json.Unmarshal([]byte(`{"type":"choice","choice":"DIRECT","probabilities":{"DIRECT":0.9,"PROXY":0.1,"UNCERTAIN":null}}`), &old) != nil {
		t.Fatal("fixture invalid")
	}
	if old.Probabilities[Uncertain] != 0 || Accept(State{Evidence: Evidence{"verified_success", "not_tested"}}, old) != Direct {
		t.Fatal("legacy coercion repro changed")
	}
}
