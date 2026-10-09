# Connectivity and policy acceptance are separate

Real run `37880640439` executed `158fc35122d0bf0be74285737071db22d9f8ed8d`. Its visible allowlisted fields report two successful DNS/direct TLS/explicit VLESS TLS checks, two HTTP 200 model responses, two `DIRECT` choices and two `UNCERTAIN` accepted decisions. Cleanup was verified; no routing update, proxy-learning acceptance or TUN test occurred. GitHub masking damaged diagnostic JSON punctuation: these are readable typed fields, not a revalidated full JSON report. No masked value was reconstructed.

## What the old result does and does not establish

The production evidence passed to the model and to `Accept` is the same `State.Evidence`. For this run it was `verified_success/not_tested`, which permits DIRECT. The separate explicit VLESS TLS check never replaces the production proxy evidence. Therefore the untested proxy evidence did not veto DIRECT.

`Jev.Decide` parses `answers.route` into the answer type, choice, probability map and confidence. A decoded answer and a valid choice are enough for `realHost` to record `model=answered`; that is not a guarantee that the distribution satisfies `Accept`.

For these observed DIRECT choices, the possible old rejection branches are:

- Answer type was not `choice` (including a missing type).
- Probability map did not have exactly the three expected labels, or a value was invalid.
- Probability sum differed from 1 by more than 0.02.
- The otherwise valid DIRECT probability was below 0.8.

The old diagnostic exposes none of those fields or branch reasons, so it cannot distinguish a normal low-probability refusal from a response-contract problem. We cannot claim that confidence was low. `Confidence` is decoded but never consulted by `Accept`.

The [OpenRouter response reference](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request) uses `type`, `choice`, `probabilities` and `confidence`. Its example distinguishes the selected probability from confidence. The [official tutorial](https://openrouter.ai/docs/guides/community/jev-tutorial) describes confidence as distribution concentration, not independent evidence of safe reachability. The existing gate continues to use the selected probability and locally collected TLS evidence, with no threshold change.

## Offline finding and minimal change

The previous direct-to-`float64` decoder coerced a JSON `null` probability to zero. Synthetic response `{DIRECT:0.9, PROXY:0.1, UNCERTAIN:null}` consequently became a valid-looking numeric distribution and was accepted with direct TLS evidence. This is a separate parser bug; it explains an unsafe possible acceptance, not the two observed refusals.

The wire decoder now retains nullability and rejects null probability values with a fixed error. Missing distributions still fail the unchanged policy gate. No value is inferred from confidence or filled in to obtain acceptance.

`Accept` and real acceptance share `acceptWithReason`. It preserves the existing decisions while reporting one fixed `acceptance_reason` per real-host result:

| Reason | Meaning |
| --- | --- |
| `not_evaluated` | The host did not reach policy evaluation |
| `answer_type_invalid` / `choice_invalid` | Answer contract rejected |
| `probability_count_invalid` | Wrong number of labels or missing map |
| `probability_value_invalid` | Missing expected label, nonfinite or out-of-range value |
| `probability_sum_invalid` | Sum outside the original tolerance |
| `evidence_insufficient` | Evidence supports neither DIRECT nor PROXY |
| `choice_evidence_mismatch` | Model choice differs from the evidence-supported decision |
| `probability_below_threshold` | Selected probability below the original 0.8 threshold |
| `accepted` | DIRECT or PROXY passed the existing gate |

Only these labels enter the diagnostic. No probabilities, confidence values, raw answers, addresses or errors are emitted. Both invalid wire responses and policy refusals remain fail-closed. The wrapper's strict output contract is updated in the same commit as the worker.

## Verification and remaining acceptance work

Offline tests cover all policy branches, normalized 0.79/0.8 distributions, confidence independence, literal external JSON fixtures, null coercion, evidence binding, explicit proxy/evidence separation, fixed-reason output and canary exclusion. The earlier low-score test used an unnormalized distribution and therefore only exercised the sum check; the new boundary tests correct that coverage gap.

No new real request is needed to validate the parser fix or deterministic policy behavior. If a later, separately approved bounded run is needed to explain a live refusal, the smallest change is to run the same targets with the fixed reason diagnostic. A `probability_below_threshold` result is a valid refusal, not a reason to lower the threshold or repeat until green.

Connectivity acceptance has succeeded for the tested targets and node. Full policy-learning acceptance remains separate: an accepted decision must come from qualifying production evidence, and the resulting provider update, acknowledgment/readback, precedence and lifecycle behavior need their own controlled acceptance. PROXY requires repeated direct failure plus verified proxy TLS, not merely a successful explicit proxy transport. This change does not expand target scope, enable publication or TUN, change main, or authorize another live run.
