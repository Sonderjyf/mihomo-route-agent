# PR5 complete-product acceptance report — preparation only

Campaign: PR5-COMPLETE-20261016-01.
Formal run ID / attempt / launcher SHA / product binary SHA256: UNKNOWN (not launched).
Result: INCOMPLETE. Keep PR #5 Draft. No merge/release/production installation.

## Preparation evidence

All product edits, gofmt, builds, Go tests/vet and acceptance guard execution
were performed by GitHub-hosted windows-2025 preparation jobs. The local session
only read public repository evidence, prepared reviewable task data and operated
GitHub. No local FlClash/core, network changes or model request was used.

| Preparation run (attempt 1) | Input source | Outcome and boundary |
| --- | --- | --- |
| 38909472084 | 15f55c3755c10db4fd45c8ecb4ec1f85b49f14b5 | Bootstrap boundary only; no task or product acceptance |
| 38910929060 | fda785aec8bc6f2c4e597a9bae346f5e81031fef | Stopped before source apply: Windows CRLF preimage mismatch; no learning |
| 38911328168 | 27a6a3239a447ff216379d692ba9681b943715f3 | Go checks and 49 guards passed; nonexistent guard pattern then failed; no product commit |
| 38911787697 | 19892427bd358d2dd77df847bc9288e82a9de04d | Go test/vet/build and 49 guards passed; VM wrote c8d124deb083cff46806d374ff94378fedff1307 |
| 38912804607 | ca46aba5bf91f86d3e9060122ef249d8f06c7e8c | Go test/vet/build and 50 guards passed; VM wrote ad3fae6d95ff4e74d0be269adb9e74c079ca083f |

Later driver preparation must be identified by its own run and exact source in
the PR; this table does not claim unobserved future checks. Prepared code and
ordinary CI are not formal acceptance.

## Formal scene gates (not executed)

| Scene | PASS condition | Actual status / evidence |
| --- | --- | --- |
| C00 | Frozen clean SHA, one binary, official app/core hashes, actual guest app/TUN capability | INCOMPLETE — capability has not run |
| C01 | Real DNS/TLS + one actual model response + unchanged production policy adoption, connection/decision IDs | INCOMPLETE — no campaign call |
| C02 | Zero nonempty submissions before due; current accepted result; fresh fetch + ACK/readback; applied publication ID | INCOMPLETE — no campaign publication |
| C03 | First fallback retained; distinct later connection ID/target/learned payload/actual DIRECT chain | INCOMPLETE |
| C04 | Natural repeated direct failures + verified proxy TLS + accepted real PROXY + timed publication/new connection | INCOMPLETE |
| C05 | Separately approved VM controlled fault, same strict PROXY evidence and restored firewall | INCOMPLETE — optional and never repairs C04 |
| C06 | Actual nonempty FlClash use, GUI refresh/switch/restart, exact config/rule/DNS identities, old ownership refused | INCOMPLETE |
| C07 | Actual native TUN traffic, transferred bytes/TLS, connection payload and DIRECT/PROXY chain | INCOMPLETE |
| C08 | Two periods/no redundant nonempty PUT; expiry/version/refusal/late/ownership guards correctly labeled | INCOMPLETE — existing offline guards are only preparation |
| C09 | Normal publisher exit after empty fetch/ACK/readback and stopped journal; no forced-stop substitution | INCOMPLETE |
| C10 | Declared maintenance pause/empty/current-generation resume | INCOMPLETE |
| C11 | Deliberate confirmed publisher exit then valid-owned empty recovery; no old-generation authority | INCOMPLETE |
| C12 | Independent cleanup, exact process/port/private-file/network restoration | INCOMPLETE — formal VM not started |

The driver reports the known subset of model attempt observations plus reserved
slots. An unobserved child outcome is UNKNOWN, never inferred as zero. It records
source/launcher/app/core/binary/config identities, safe decision and publication
IDs, source/later connection IDs, exact payload/chain, provider ACK/readback and
per-action cleanup results. Private configs, API keys/node credentials, raw model
responses and the private decision ledger are never uploaded or printed.

## Budget and authorization

Known model transport attempts in these new secret-free preparation runs: 0.
Formal campaign model attempts, spend and cleanup: UNKNOWN / not executed.
Proposed total model transport limit: 6; metadata GET limit: 2.
Approved campaign USD / existing restricted-key cap / verified current remaining
allowance / current spend / approved node endpoint and traffic allowance:
UNKNOWN. Historical USD 1 is not reusable authority. No account limit reset.

Before activation use CAMPAIGN_PLAN.md and the disabled launcher to obtain one
consolidated authorization covering the final full source SHA, launcher/build
provenance, fixed six-slot scope, exact target/resolver/API/node range, verified
budget facts, actual VM FlClash/TUN/split-route/firewall/provider writes and
independent stop/recovery. The normal active workflow remains disabled.

## Historical boundaries retained

Run 37887426801 used launcher 755679b0019ef7d605a30c8bfe01b5156c2dbe86
and executed b4f6f18fbbac6edb83093484934fe8d2e8448f07:
two HTTP 200 model responses, policy probability refusal, no learned publication.
Run 37880640439 refusal cause remains unknown. Failed 37867539248 spend, attempt
count and cleanup remain unknown. Do not reconstruct missing data.

Historical empty-provider FlClash run 37740165846, synthetic TUN 37722276208 and
lifecycle 37753376144 retain their original evidence files/SHAs and scope.
They do not fill any current formal scene. Maximum proposed active observation
is twelve minutes; no long-term stability acceptance is claimed.
