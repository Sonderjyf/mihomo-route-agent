# PR5 complete-product campaign (not executed)

Campaign ID: PR5-COMPLETE-20261016-01.
Status: PREPARATION, not an execution authorization.
One manually admitted GitHub-hosted windows-2025 job, attempt 1 only.
No rerun, retry after failure, extra target, replacement model, weaker threshold,
budget increase or timeout extension is permitted. Ordinary secret-free CI is
separate and never counts as this campaign.

## Admission and immutable provenance

Before enabling any launcher, fill the final full source SHA, launcher SHA,
clean-checkout identity, Go 1.25.10/action SHAs, dependency checksums and SHA256
of route-agent.exe once. The disabled candidate is
[docs/github-complete-campaign.yml.disabled](docs/github-complete-campaign.yml.disabled),
and the inert driver is scripts/complete_campaign.py. Python is 3.13.7;
requirements-smoke.txt fixes dnspython 2.8.0 and PyYAML 6.0.3. Download binary wheels
before Secrets, record their names/SHA256, then install only those local wheels.
Go module integrity uses the checked-in go.sum. No dependency download is allowed
in the execution step. Reject dirty/untracked replacements. Build one product
binary in a no-application-secret step and use that same artifact throughout.
No injected Go test worker can stand in for product behavior.

Use a manifest listing each scenario's planned core/app and effective-profile
version/digest before any model call. All formal runtime scenes use the official FlClash app and its bundled core;
standalone Mihomo is not substituted. Official FlClash 0.8.99:
archive 9ff3a9315b51e6665669bfde9b0323ddb671a22d0a9647987b4cc5e574007f4f,
source 68c71b8ef9b7486a224972eb371ff153c6b2de0f.
App executable SHA256:
9f72ed125300750e4170f1a5424118f7c4694a9e1c035977817ce8d52e4c10c9.
Bundled core SHA256:
f291a9695368586ec19390c437b95e33479fe3f365d5b3f837acdc391e099c3a.
Verify those bytes in the new VM; their historical record is not a new execution.
Never substitute standalone core or use the version string as binary identity. Profile A/B refresh and core
restart are deliberate, predeclared identity transitions, not hidden swaps.
A new generation must start empty with a new lease/journal/session.

Secret-free admission checks must establish an interactive active desktop,
English OCR, administrator capability, isolated unused ports and clean guest
routes/DNS/proxy/process baseline. Verify official app/core dependencies and TUN
driver prerequisites. An unsupported hosted image is a preparation blocker.
Actual TUN creation/transport must pass its approved guest-only capability gate
before any model step. enable=true, a driver file, historical support or mocks
cannot pass that gate. No local/Sera production device is an alternative.

## External scope and budget facts still required

Proposed exact TLS targets: example.com:443 and www.cloudflare.com:443.
Independent resolver: 1.1.1.1:53, UDP with bounded TCP fallback.
Model: typesafe/jev-1.13 at
https://openrouter.ai/api/alpha/decisions, no alternate endpoint/model.
Additionally authorize at most TWO GET requests to https://openrouter.ai/api/v1/key:
one before model traffic, one only after every scenario succeeds. Read only the
existing key's numeric limit, remaining allowance, usage and reset status; never
create/update/reset a key. A mismatch or periodic-reset key blocks paid calls.
See the [provider key metadata contract](https://openrouter.ai/docs/api/api-reference/api-keys/get-current-key).
The final usage delta is an observed counter, not a final settled invoice; unknown
final billing stays unknown. No metadata GET follows a failed scenario.
Only the already approved single six-field VLESS_NODE_JSON node may be used.
The owner must identify/approve its endpoint and traffic allowance without
publishing credentials or subscription URLs.

Proposed maximum: SIX total model HTTP transport attempts across the entire
campaign, including failed/timeout/availability requests, never six per scene.
The first request is a learning attempt, not a free connectivity precheck.
Slots are fixed before launch: (1) app-A real DIRECT, (2) app-A naturally
eligible PROXY, (3) separately labeled controlled PROXY fault if approved,
(4) final app-B generation real DIRECT after refresh/switch/restart,
(5) unchanged-generation maintenance-resume real DIRECT, and (6) a fresh
owned session's real DIRECT before the deliberate publisher-crash recovery. A slot is not an obligation:
failed evidence means no model call. A policy refusal is NO-GO for that learning
scene and must not be retried. No attempt budget resets on child restart,
maintenance or a new ownership generation.

The numeric approved USD cap, existing restricted-key cap, verified current
remaining allowance, present spend and node allowance are UNKNOWN.
Historical USD 1 is not available authority and must not be reset.
No paid call can begin until all required facts are verified/attested and the
single consolidated authorization covers the final SHA and every action.
A count is not a dollar cap. Missing billing evidence stays unknown.

## Scenario order and PASS gates

| ID | Scenario | Required same-campaign evidence |
| --- | --- | --- |
| C00 | Clean provenance and capability admission | Exact manifest, fixed artifact hashes, clean checkout, first run attempt, private isolated paths, free ports and network baseline; usable actual app/TUN prerequisites |
| C01 | Real DIRECT judgment | First eligible TCP443 connection uses original terminal fallback; real independent DNS and verified TLS; actual one-attempt model result; unchanged 0.8 probability threshold, 0.02 sum tolerance and <=10s production evaluation; suggestion distinct from adoption |
| C02 | Timed DIRECT publication | Decision/connection ID receipt pending; nonempty submission counter zero before first_due; first nonempty submission timestamp >= due; only current unexpired adopted scope; fresh fetch, each provider ACK and metadata/count readback; receipt applied with publication ID |
| C03 | Later DIRECT connection | A new ID to the same target/address/443 hits the exact learned AND payload and actual DIRECT chain; original A retains its ID/fallback/chain; original rules, other ports and Fake-IP mapping preserved |
| C04 | Natural PROXY learning | Real repeated direct failures plus verified proxy TLS to the same independently resolved public address; only then actual model call and natural policy adoption. High probability or a separate VLESS smoke does not replace evidence. Timed publication and a distinct later learned PROXY connection must correlate |
| C05 | Controlled PROXY fault (separate claim) | Only if explicitly approved: target-scoped guest firewall fault on the direct path, no default route/proxy/DNS changes; actual production repeated failures and verified proxy TLS. Restore and verify exact firewall baseline independently. Never label this natural PROXY need or use it to repair C04 |
| C06 | Nonempty FlClash integration | The actual official app generates/uses the campaign's HTTP providers and exact TCP443 tails; runtime fetched nonempty contents agree. Preserve complete ordered original rules and all DNS fields. Drain/exit before actual GUI refresh A->B, repeated B, profile switch and restart; verify each expected config/core identity, no duplicate tails or old-lease writes. Relearn only within the preallocated fresh-generation slot |
| C07 | Actual TUN transport | Guest-only split routing for the approved Fake-IP prefix keeps real evidence/control/node routes physical; ordinary native socket enters actual Tun inbound, uses the same target mapping, learned payload and DIRECT/PROXY chain, with transferred bytes/TLS evidence. Config flag alone fails. Record actual adapter/route and connection observations |
| C08 | Timed/no-op/expiry/version guards | Observe at least two publish periods; unchanged snapshots have no additional nonempty submissions. Expired, refused and late results never enter payloads. Agent-policy/rule/profile/core generation or lease loss prevents writes; do not fabricate model probabilities for these deterministic guard scenes |
| C09 | Normal stop | Normal process exit only after verified empty provider fetch/ACK/readback, stopped journal and matching final summary. Verify owned app/core/helper processes and ports gone. Forced stop or later recovery cannot pass normal exit |
| C10 | Maintenance and same-generation resume | Explicit UTC+08 window/start/duration/resume policy in manifest; pending/applied withdrawal only after empty ACK, no nonempty writes while paused. Resume unchanged generation only after fresh checks. Changed profile/core refuses old ownership |
| C11 | Abnormal publisher exit and empty recovery | One deliberate owned publisher crash, confirmed exact process absence before empty-only recovery; current ownership valid, fresh fetch/readback empty, no automatic learning restart. Refuse recovery on unconfirmed exit or changed generation |
| C12 | Independent cleanup and final guest restoration | Attempt each cleanup independently even after another fails: close sockets, drain/recover only under valid ownership, stop recorded children, remove guest fault rules/TUN routes/adapter, remove owned private files, verify all ports/processes absent and exact proxy/DNS/default-route/firewall restoration |

C06/C07 require real nonempty results produced under the respective current
ownership context. Reusing historical empty-provider app evidence, a synthetic
TUN worker, an old decision across a profile change or a different binary is not
an accepted shortcut. All deliberately synthetic/version/late/ownership guard
scenes must be labeled as deterministic fault tests and remain separate from
real policy adoption evidence.

A rejected learning result is normal fail-closed behavior, but it is NO-GO for
that scene. Stop its dependent publication/connection/app steps. Once a failure
requires stopping external execution, do not make more target/API/node requests;
continue independent cleanup and collect only evidence already available without
external calls. No workflow success badge overrides scene results.

## Bounded observation and cleanup

Proposed job maximum: 40 minutes, including a reserved final five-minute cleanup
window. Maximum active observation: TWELVE minutes, including at least two 30s
publication periods and the declared short maintenance window. This is bounded
observation, not long-term stability acceptance. Each network/model operation
keeps its production bounds; total spending and attempt limits are independent.

A separate always-run cleanup step must not depend on the success of the secret
execution step. The child environment is allowlisted; only the product observer
receives the model key, only the owned profile/core setup receives node input.
No raw model response, private config/ledger, node credentials or API keys are
printed/uploaded. No secrets are supplied to PR CI, builds or dependency setup.
No service, system startup task, release, merge or production configuration change.

## Report contract

For every C00..C12 row record PASS / NO-GO / INCOMPLETE, run ID/attempt, source
and launcher SHAs, product/core/app hashes, config/generation digest, observed
time and evidence origin. Correlate target index and approved same-run address
identity, first/subsequent connection IDs, decision ID, policy adoption/status,
publication ID, provider payload digest/fresh fetch/ACK/readback and actual chain.
Publish only allowlisted redacted fields, never the private source ledger.
Missing facts are unknown, not zero or reconstructed.

Report exact known model transport attempts, attempted-but-unknown outcomes,
spend/remaining allowance with evidence timestamps and each cleanup result.
Keep historical evidence under its original launcher SHA versus execution SHA:
run 37887426801 launcher 755679b0019ef7d605a30c8bfe01b5156c2dbe86,
executed b4f6f18fbbac6edb83093484934fe8d2e8448f07; it accepted no learned rule.
The earlier 37880640439 refusal cause and failed 37867539248 spend/cleanup remain
unknown. Historical runs never fill this campaign's missing rows.

Ready requires all required complete-product rows to PASS, confirmed normal stop
and cleanup, no unknown provenance/budget facts and an explicit remaining-limit
statement. A natural PROXY failure, missing actual TUN/app integration, or only
synthetic fault coverage leaves the corresponding gate NO-GO/INCOMPLETE and PR
Draft. No merge or release is authorized.
