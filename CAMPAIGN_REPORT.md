# PR5 complete-product acceptance report — preparation only

Campaign: PR5-COMPLETE-20261016-01.
Formal run ID / attempt / launcher SHA / product binary SHA256: UNKNOWN (not launched).
Result: INCOMPLETE. Keep PR #5 Draft. No merge/release/production installation.

## Current result (2026-10-17)

The formal campaign has NOT started. Pre-campaign budget admission is NO-GO;
overall complete-product acceptance remains INCOMPLETE. Source candidate stays
6c4e2689af4074b579f21cb52a7a61f226e3e4d3. Report/workflow-preparation commits
are not silently substituted as a new acceptance binary. C00-C12 remain unrun.

The owner explicitly approved the two minimal trusted-main entrypoints and
conditional execution, without merging PR5 or changing environment protection.
This resolves the previous automatic-review main-write rejection. Only the
budget entrypoint was installed: launcher commit
6d35ea11e1608feab474c9a03cba9d0804afab8e, exact workflow content SHA256
f900e0c968e5d76a1634d25b24ef591e1001ebfad91bd09100535d2acadc900a.
The conditional formal launcher was not installed or dispatched because numeric
budget admission did not pass. No retry, rerun, alternate model or extra target.

[Budget admission 38934621245](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/38934621245),
attempt 1, job 117086870013, ran on GitHub-hosted Windows, checked out the exact
6c4e2689af4074b579f21cb52a7a61f226e3e4d3 source, and used only the existing
protected-environment Secrets. Observed output validates the existing six-field
node JSON and its canonical binding SHA256
249a87932186adf39c8a3c06eb3c33ae68008beebf2f5d583345877f2041fc2f.
This is schema validation, not node connectivity/TLS/traffic acceptance.

The later key-metadata stage returned READ_ONLY_ADMISSION_NO_GO. The launcher
suppressed exception details and did not emit numeric cap/remaining/usage or an
observed HTTP status. Cause, actual HTTP outcome, account cap, remaining balance,
reset period and invoice/spend therefore remain UNKNOWN. Do not infer budget
exhaustion, HTTP 200, request arrival or parse success from workflow failure.
There was one bounded metadata-GET call site after node validation; transmitted
and completed-request counts cannot be established by the retained diagnostics.
No model POST exists in this launcher: model transport attempts are known 0.
It contains no account-write/reset request, app/core launch, TUN, firewall,
provider write or real-node network use. No account limit was changed.
The failure does not authorize another Secret/API diagnostic run.

Checkout post-cleanup and job completion succeeded. No private configuration,
model response, node credential, private ledger, owned app/core/network process,
route, firewall rule or provider was created by this read-only job. There is no
formal VM cleanup result to claim. Actual formal binary/config identities,
judgment/publication/provider/connection correlations and active observation
remain UNKNOWN/not executed. Maximum planned observation is twelve minutes;
actual formal observation is zero, and long-term stability is untested.

The three separate PR synchronization preparations 38933948861 (fc021001...),
38934459445 (3d0edc52...) and 38934497489 (05560cc4...), all attempt 1, were
automatically created by PR synchronization while the PR-only trigger existed.
Their annotations independently confirm rejection of refs/pull/5/merge before
runner/Secret access. Metadata/model requests from these jobs are known 0.
They are not real-model attempts and do not fill any campaign scene. The PR-only
automatic trigger was retired at ee4cd2145b4d238ce2b5a91b260cccd58122bf56;
its retained history is not rewritten into success. No environment rule changed.

No paid-model request was made by any new preparation. Whole-account historical
spend is unknown. The USD 1 authorization is current but cannot replace missing
provider cap/remaining evidence. Further external work requires those missing
facts and an explicitly bounded next action; it must not be an automatic retry.
PR5 remains Draft.

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
| 38922339850 | 4e0f378d1170865c8e9b20ed91e5eb2112f2b397 | Go test/vet/build and 55 guards passed; VM wrote inert full driver 40d5c9ad5ee10d0b8527045c5a8f6d54b79308f4 |
| 38923161346 | fd93e4c7513cb4e925168c9d9b846060748727a2 | Go test/vet/build and 56 guards passed; VM wrote ACK-validity and per-call observed HTTP diagnostic f8bc2cb2e543209492d71524b9e01c8434a64f7b |
| 38924584970 | 8a1bfc908b8d7842bc4645cf600a53d9886a8c6b | Go test/vet/build and 57 guards passed; VM wrote 2f5013bbca40161e03da0e7ff2002b29d0620959; no real/model/app/TUN execution |
| 38925074950 | Head 6c4e2689af4074b579f21cb52a7a61f226e3e4d3; merge 0fe88298e1f479413fd8c92b1fd523a20a27cddc | Ordinary Windows/Linux CI passed; identical file tree; Linux race and Windows package smoke only |
| 38933948861 | Workflow fc021001ac87e866f8f03f229c1dbd0bb7ba9409; pinned source 6c4e2689af4074b579f21cb52a7a61f226e3e4d3 | Environment protection rejected refs/pull/5/merge before runner/Secret access. Metadata GETs 0, model transports 0; no app/core/network changes or rerun |
| 38923789456 | 9922fa8d03622361c4342cbec69d2d93ac77aa6b | Stopped on existing Gate test TCP bind denied for UDP-selected port; no task source commit, no rerun or model attempt |

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
per-action cleanup results. Only the public report JSON may be retained as a seven-day immutable GitHub
artifact after independent cleanup. Log masking is never reconstructed.
Private configs, API keys/node credentials, raw model
responses and the private decision ledger are never uploaded or printed.

## Budget and authorization

Known model transport attempts in these new secret-free preparation runs: 0.
Formal campaign model attempts, spend and cleanup: UNKNOWN / not executed.
Proposed total model transport limit: 6; metadata GET limit: 2.
Minimum planning reserve is USD 0.10; it is not an account credit or authorization.
Owner authorization on 2026-10-17: new campaign ceiling USD 1, reported no
periodic limit, existing GitHub Secrets/VPS node, controlled guest firewall and
comprehensive GitHub Actions testing. This is current owner authority, not reuse
of the historical USD 1. Actual existing restricted-key cap, remaining allowance,
reset period and current spend: UNKNOWN. Node schema and binding were subsequently validated as recorded above; real node connectivity remains UNKNOWN.
GitHub Secret metadata confirms OPENROUTER_API_KEY and VLESS_NODE_JSON under
real-acceptance-manual-approval; no repository Secrets. No Secret plaintext was
read locally. No account limit was reset or changed.

Read-only run 38933948861 was blocked by the existing environment's branch
protection before runner/Secret access. It made zero metadata GETs and zero
model transports; no private process/file/network cleanup was required. Do not
rerun it or weaken the environment protection. Automatic approval review rejected
adding the minimal budget entrypoint directly to main as outside the PR-branch
boundary; no main commit was made. Explicit trusted-main-entrypoint permission
was subsequently granted; the numeric admission then failed as recorded above. The read-only candidate is
[github-budget-admission.yml.disabled](docs/github-budget-admission.yml.disabled).
Its exact content SHA256 is f900e0c968e5d76a1634d25b24ef591e1001ebfad91bd09100535d2acadc900a.

A separate budget admission, if explicitly authorized on main, adds at most one
read-only metadata GET and no model request. The formal campaign retains at most
two metadata GETs and six model transports; combined maximum metadata GETs is
three. Admit only a verified nonperiodic existing cap <= USD 1 and remaining
allowance >= USD 0.10; never change/reset a cap to satisfy admission. A candidate
formal launcher may bind the exact six-field node to the canonical private JSON
SHA256 from successful admission, deriving its endpoint only inside the Secret
step. This exposes no endpoint/credential or private YAML. The budget launcher was subsequently installed/executed once on main as recorded above; the formal launcher remains uninstalled.

Before activation use CAMPAIGN_PLAN.md and the disabled launcher to ensure
applicable authorization covers the final full source SHA, launcher/build
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
