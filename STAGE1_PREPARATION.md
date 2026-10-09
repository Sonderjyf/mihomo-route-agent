# Stage 1: production diagnostics and inert learning acceptance

Stage 0 remains the evidence baseline. This change prepares one future learning
run; it does not execute it, activate a launcher, change a Secret, or establish
production usability. All external acceptance is restricted to GitHub hosted
`windows-2025`. Sera's proxy, FlClash, DNS, TUN and routes remain outside scope.

## Production result contract

`EvaluateEvidenceDetailed` is the shared collector/model/policy entry point.
`EvaluateEvidence` remains a compatibility wrapper. The 0.8 selected probability,
0.02 sum tolerance, TLS evidence requirements, JSON-null rejection, ten-second
evaluation maximum, observer preflight limit and publication deadline remain.
`probe` adds `evaluation` to its existing result and emits it on evaluation error.
The existing probe `state` remains private operator output, including hostname.

Observer `/status` adds one `evaluation` object (initially null); it keeps only
the latest completed job, with no hostname history. The CLI also emits a final
`{evaluation, observer_stopped}` summary after bounded observe/observe-apply exit,
including failure. No summary after a kill means unknown, never zero attempts.
The diagnostic is version 1 and contains only:

- Fixed stage/error/policy labels; legal choice and accepted decision; coarse
  evidence/DNS labels. Parser/transport errors have `policy_reason=not_evaluated`.
- Effective remaining evaluation/job deadlines, monotonic elapsed milliseconds,
  DNS, physical-path checks, direct/proxy TLS, collection, model and publication
  spans. Milliseconds are bounded integers; round-down can report zero. Spans
  include local overhead and are not reconstructed from old workflow timings.
- Judge calls separately from transport attempts and an explicit known flag.
  Production Jev uses fresh HTTP/1 connections and disables redirects to prevent
  hidden POST replay. Counts are transport attempts, not proof of delivery or a
  monetary cap. A failed attempt can still be charged. Stub/injected clients do
  not invent known transport counts.
- Publication not_attempted/failed/committed separately from policy acceptance;
  committed means the existing fresh-fetch/ACK/readback checks completed.
- `connection_target_match`: only a boolean comparing the triggering core TCP
  connection's destination IP to the collector's pinned DNS address. It does not
  change policy gates. Missing/different metadata cannot pass learning acceptance.

Neither this object nor the future public report contains raw response,
probabilities, confidence, hostname, IP, credentials or arbitrary error text.
`scripts/learning_acceptance.py::diagnostic` enforces exact keys, enums and ranges.
Existing real-acceptance output/schema and its historical logs remain unchanged.

## Inert harness and disabled launcher

Safe offline inspection:

```console
python -B scripts/learning_acceptance.py
python -B -m unittest discover -s scripts -p test_learning_acceptance.py -v
```

Default invocation only prints the version-1 inert plan; it does not inspect
Secrets, invoke git, download, bind a port, start a process or write Controller.
`prepare`/`execute` require the trusted main learning launcher, first attempt,
GitHub hosted Windows, explicit execution permission, a literal full-SHA
allowlist, clean exact-SHA checkout, spending/node approval, confirmed remaining
provider allowance and a VM-derived physical interface. Other contexts refuse
before reading Secret values or starting infrastructure.

`examples/github-learning-acceptance.yml.disabled` remains outside workflows,
uses `if: false`, and has an invalid `REVIEW_REQUIRED_FULL_SHA` placeholder.
This deliberately cannot run until separate review and authorization pin it on
main. It reuses environment `real-acceptance-manual-approval` and its existing
`OPENROUTER_API_KEY`/`VLESS_NODE_JSON`. Neither value is copied locally. Only the
execute step receives Secrets; only the product observer receives the model key.
No PR label, push, rerun, schedule or ordinary CI authorizes it. The active CI
change adds only mocked offline learning guards.

The harness reuses the official Mihomo 1.19.32 download/archive/binary hashes,
strict node schema, budget gate, child-environment allowlist and owned-process
cleanup from the existing real acceptance. Preparation builds `cmd/route-agent`
as `agent.exe`, not an injected Go test worker. Execution is one target
`example.com:443`, one eligible connection and at most one model transport
attempt; TUN and DNS interception stay off. The independent resolver is
`1.1.1.1:53`; baseline and API egress use the existing approved VLESS node.
No destination HTTP request is sent, only CONNECT/TLS. No continuous mode,
retry, model change, probability lowering or automatic timeout extension exists.

The VM resolves that target's A record at the approved resolver and pins the
actual same-run public address in the temporary core's `hosts` map. This is
application configuration only, never Windows/Sera hosts or DNS settings. The
Collector still independently resolves the hostname and must match that address;
it receives no injected evidence/address. A mismatch is NO-GO. In official
[Mihomo 1.19.32 TCP handling](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/tunnel/tunnel.go),
hosts mapping preserves observed hostname metadata and supplies numeric dial
metadata; [Metadata.Pure](https://github.com/MetaCubeX/mihomo/blob/v1.19.32/constant/metadata.go)
removes the dial hostname for that mapping. This avoids proxy-side DNS silently
choosing a different address. These code contracts still need same-run live
connection verification; this preparation does not claim that verification.

Two namespaced providers are HTTP/classical/yaml, fetched through DIRECT on
loopback with controlled private paths. The exact two TCP443 AND tail rules and
final MATCH are compared with product `tail-preview`; the empty file preview
is never passed off as a runtime HTTP configuration. Fixed VM ports are checked
for both TCP and UDP conflicts before startup. The finite observer lasts 70
seconds (below its ownership lease); the hosted job limit is 20 minutes.

## Future acceptance criteria

PASS requires, in the same run:

1. The source-port-associated first TLS connection A matches the original
   MATCH/VLESS baseline, and the real collector target matches A's destination.
2. Production policy naturally accepts DIRECT within its unchanged budget;
   one known model transport attempt and one commit, one direct cached rule,
   zero proxy cached rules, with production fresh fetch/ACK/readback completed.
3. A distinct TLS connection B has the same hostname/address/port and matches
   the direct tail AND payload and DIRECT chain. A's original ID/MATCH/chain
   remains visible. Missing short-lived connections fail correlation.
4. Normal finite exit reports the stopped journal, empty provider readback and
   a valid final stopped summary matching the evaluated runtime status;
   owned bootstrap/agent/core, private files and ports are verified clean.

Refusal with a fixed reason, zero commits and verified empty cleanup is NO-GO,
not learning PASS. Deadline/path/publication failure is separately reported.
Unknown model attempts, invalid/missing final summary or failed/unknown cleanup
remain INCOMPLETE. Same-run address mismatch never becomes PASS by combining
different DNS answers. Empty recovery after confirmed publisher exit remains
owned/unchanged-generation only; locks are not deleted to bypass checks.
Successful forced cleanup/recovery cannot upgrade a failed normal stop to PASS.
Each cleanup action runs independently; no raw logs/configuration are uploaded.

This phase adds two focused Go diagnostic/cancellation groups and four Python
guard groups. Existing policy/parser, publication, ownership, lifecycle and
ordinary Windows/Linux CI checks are reused. Actual harness success, real rule
learning, FlClash nonempty integration, current TUN and daily operation remain
unaccepted. No new real run was performed, and no old UNKNOWN was backfilled.
Independent source review found a PASS classification bug after successful
connection checks but failed normal exit. A fully mocked regression reproduced
seven false-PASS cases; the correction requires normal exit, stopped journal,
empty readback and matching strict final summary before PASS. Missing/invalid
summaries and successful recovery after failed exit remain INCOMPLETE.

## Next approval material

After ordinary CI and source review, use the final full commit SHA and its
product/package provenance to fill the disabled launcher. A separate approval
must identify that SHA, one run/one attempt, target/resolver/node/API scope,
provider account's existing cap and verified remaining allowance after past
runs, traffic costs, VM-only empty/nonempty provider writes and cleanup, 70-second
observer/20-minute job, TUN off and no Sera configuration changes. Do not reset
the existing account limit or assume the historical USD 1 is still available.
Refusal/timeout stops the experiment; approval does not authorize retry.

The existing runs 37880640439 and 37887426801 already establish their scoped
connectivity/model outcomes. Do not repeat their two-target diagnostic campaign.
