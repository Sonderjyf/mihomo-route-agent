# Delivery checkpoint: connectivity passed, real learning not accepted

This is an experimental Windows amd64 package, default disabled. It is not a complete production acceptance or permission to change an installed FlClash/profile/network. Start with [PORTABLE_QUICKSTART.md](PORTABLE_QUICKSTART.md); operation, stop and recovery details remain in [RUNBOOK.md](RUNBOOK.md) and [RECOVERY.md](RECOVERY.md).

Stage 0 (2026-10-09) closes evidence/documentation alignment only. This page is the current acceptance source; historical sections elsewhere retain their original SHA and limits. The checkpoint is **L0: connectivity and refusal verified, real learning unaccepted**. PR #5 remains stacked on `codex/go-route-agent-prototype` at `c75166913af788107eece66406f9e16ee6ceecb2`; experiment-code review/merge and production acceptance are separate decisions. See [stage-0 closeout](STAGE0_CLOSEOUT.md).

## Product and packaging revisions

The delivered Go product code matches `b4f6f18fbbac6edb83093484934fe8d2e8448f07`: null model probabilities are rejected, fixed policy refusal diagnostics are available, and the original 0.8 selected-probability threshold remains unchanged. The final documentation/packaging commit is recorded as `revision` in `manifest.json`; `product_code_revision` records the last product-source change. Packaging verifies that `cmd`, `internal`, `go.mod` and `go.sum` match that product revision. The binary is built from the final clean packaging checkout, so its embedded VCS revision is the packaging revision, not a claim that older real tests ran on that later commit.

The ZIP includes only the Agent executable, tracked public documents, explicit synthetic/off examples and allowlisted evidence. It contains no Mihomo/FlClash binaries, API key, controller credential, real node JSON, source node YAML, private session state or raw real-run log. `SHA256SUMS` covers package files; the ZIP has a separate `.sha256` file. Runtime defaults are off with empty controller/API proxy. The maintenance template deliberately fails validation until the operator chooses its window and resume policy.

## Evidence boundaries

| Gate | Exact recorded evidence | Accepted scope | Not established |
| --- | --- | --- | --- |
| Real connectivity and policy diagnosis | [37887426801](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37887426801), attempt 1; `b4f6f18fbbac6edb83093484934fe8d2e8448f07` | Both targets: DNS, direct TLS, explicit VLESS TLS; two model HTTP 200 responses; inventory 1139/15000 ms; cleanup verified | Both DIRECT choices were refused as `probability_below_threshold`, leaving UNCERTAIN. No learned publication, proxy-learning acceptance or TUN |
| Earlier real connectivity | [37880640439](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37880640439); `158fc35122d0bf0be74285737071db22d9f8ed8d` | Same connectivity components succeeded | Its refusal branch was not logged and cannot be inferred retrospectively |
| Earlier failed real attempt | [37867539248](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37867539248); `08ec5ae340d06838f98bea7eca61c73ae921adb2` | Preparation prerequisites passed; execute failed | Root cause, actual model attempts, spend and cleanup remain unknown; later success does not repair this record |
| Native core TUN with synthetic evidence | [37722276208](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37722276208); `bf8fc3945736687646e16ff40b26083269172ac7` | Actual owned TUN transport, rule priority, learning/lifecycle assertions and guest network restoration | Not real external reachability/model evidence; not FlClash or a TUN rerun of this package |
| FlClash application configuration | [37740165846](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37740165846); `89fd846f01b2956ed80afe8eea060f964e5ad9f7` | Pinned app, synthetic subscription, empty providers, DNS/config preservation, refresh/repeat, restart and exit | Not nonempty real learning, coordinated maintenance/recovery or TUN |
| Supervised synthetic lifecycle | [37753376144](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37753376144); `39bea08f169d4098fd8884a097b628b03f35f9aa` | Synthetic learning/scope, maintenance emptying/resume, supervisor death and independent recovery | Not real model/network learning, FlClash integration or TUN |
| Native Windows identity regression | [37805487845](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37805487845); PR head `92f56b5fcdf3363384bdd26307e2e2c65490e8b8`, CI checkout `1960299a29298559574053c568d3396495691789` | Native identity/exit mechanisms on owned loopback test processes with a three-second caller budget | Not a new core/app/TUN lifecycle campaign; physical-route validation remains a separate PowerShell check |
| Ordinary offline CI | [37881557983](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37881557983); PR head `b4f6f18fbbac6edb83093484934fe8d2e8448f07`, CI merge checkout `3628de8` | Windows/Linux test/vet/build, Linux race, offline guards and Windows package smoke | No paid model, real target, app or TUN acceptance; Windows race was skipped |

Committed JSON summaries are under `evidence/`; entries without a separate JSON summary retain their linked run/log and diagnostic-document source. The real summary contains only previously validated visible enum/numeric fields. GitHub masking damaged JSON punctuation; no masked material was reconstructed and the complete JSON was not independently revalidated. Historical synthetic tests remain valid for their exact revisions; later packaging does not upgrade or invalidate their scope.

### Capability acceptance matrix

| Capability | Current status | Evidence and remaining boundary |
| --- | --- | --- |
| Model suggestion | Verified for the two real targets | HTTP 200 / answered / DIRECT in both successful real runs; suggestion is not policy adoption |
| Real DIRECT policy adoption | Not accepted | `b4f6f18` choices were refused as `probability_below_threshold`; `158fc35` refusal is `reason_unknown` in this retrospective summary |
| Real PROXY policy adoption | Not tested | Direct TLS succeeded; production proxy TLS stayed `not_tested`. Explicit VLESS TLS does not replace that evidence |
| Provider publication | Synthetic/local scope passed; real nonempty publication unaccepted | Historical synthetic lifecycle includes ACK/readback; successful real workers publish no rules and report `routing_updated=false` |
| Real learned connection | Not accepted | Historical synthetic first-MATCH/later-tail behavior passed; no accepted real model decision has been tied to a learned real connection |
| FlClash integration | Empty-provider configuration scope passed | `89fd846` app config/refresh/restart/exit only; nonempty learning with maintenance/recovery remains unaccepted |
| TUN | Historical synthetic native transport passed | `bf8fc394` owned guest TUN only; no TUN in either successful real run, and no current real-model/app/TUN combination accepted |
| Maintenance | Historical synthetic scope passed | `39bea08` empty drain/unchanged resume; no app-integrated or real-model acceptance |
| Recovery | Historical synthetic scope passed | `39bea08` parent drain/child-crash empty recovery; changed app/core generation and real learning integration remain open |
| Continuous operation | Historical synthetic supervisor scope passed | `39bea08` lifecycle plus offline clock/process checks; no daily production soak or automatic FlClash refresh owner |

Worker `completed`, HTTP success, policy `accepted`, provider commit and connection effect are distinct outcomes. The real worker checks transport/model completion and never calls the publisher. Provider ACK/counts also cannot substitute for connection rule/chain evidence. Async learning leaves the first and already-established connections on their original fallback.

## Normal refusal and remaining work

`probability_below_threshold` is an expected fail-closed result. The model's DIRECT choice did not reach the selected probability threshold; confidence is not substituted for it. We did not lower the threshold, add targets, repeat until accepted or publish a rule. The explicit VLESS TLS success is separate from production proxy evidence, which remained `not_tested` after direct TLS succeeded. Details: [policy diagnostics](docs/POLICY_ACCEPTANCE_DIAGNOSTIC.md).

No further API call is needed just to close this delivery milestone. Before future real learning/publication acceptance, the necessary conditions are:

1. An explicitly approved, exclusively owned target environment, validated core/profile identity, budget and operator-selected maintenance/stop/recovery procedure.
2. Qualifying current evidence and an accepted policy decision. DIRECT requires verified direct TLS; PROXY requires repeated direct failure plus verified proxy TLS. A high model score cannot replace those facts, and synthetic failure does not prove real PROXY need.
3. A controlled nonempty provider update with acknowledgment/readback, original-rule precedence, correct connection path and bounded lifecycle/cleanup. FlClash profile refresh/switch/restart and coordinated maintenance/recovery need integration evidence in that environment.

Production `EvaluateEvidence` allows at most ten seconds for collection/model work and respects the observer's shorter configured deadline; the real acceptance worker gave the model forty-five seconds separately. Existing connectivity success does not accept the production deadline. `tail-preview` produces offline file providers, not the HTTP providers required by the controlled publisher. Neither gap is changed by stage 0.

Offline policy, publication and lifecycle fixtures can continue to test deterministic behavior without spending API budget. A real run is justified only by a specific remaining acceptance question and separate authorization, not by a desire to turn a normal refusal into a pass. This package does not install a service, enable autostart, activate TUN or change the host proxy/DNS/routes.
