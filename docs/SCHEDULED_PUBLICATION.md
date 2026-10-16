# Judgment records and periodic publication

Controlled observe-apply and run-controlled now separate judgments from periodic
publication. A policy-adopted judgment records pending work and never calls
Providers.Change. Shadow remains read-only. Synthetic observe-lab and the older
DNS-Gate experiment retain their original behavior and cannot substitute for
acceptance of the controlled product.

publish_interval_seconds defaults to 30 and accepts 1..3600. The monotonic period
starts after verified empty bootstrap. A busy serial worker skips missed ticks;
there is no hard real-time publication promise or backlog replay. Judgment work
retains the existing maximum ten seconds and unchanged policy/evidence gates.
Publication uses a new tick context with the existing ten-second operation bound,
parent lifetime, current lease and current ownership checks. A completed
judgment context grants no future permission.

Controlled validity remains at most 60 seconds, counted from evaluation start.
A period longer than validity can deliberately publish nothing. Every tick
checks the private lease, profile digest, core PID/start, ordered runtime rules
and physical routes. Every PUT including rollback repeats ownership validation;
nonempty PUTs reject expired entries. Scope includes the current policy version,
agent configuration, immutable ownership generation and rule digest. Renewal of
expiry alone is allowed; changed generation or policy is not.

Only adopted, unexpired, same-scope DIRECT/PROXY records enter snapshots.
UNCERTAIN, errors, late results and superseded duplicates cannot become rules.
Duplicates cannot extend existing validity. Identical snapshots cause no PUT.
The existing poll still promptly evicts expired applied entries between ticks;
it cannot promote pending results. Maintenance and stop also drain between ticks.
Original rule order, Fake-IP meaning and existing/first fallback connections
remain unchanged; no established connection is migrated.

Fresh payload fetch, PUT ACK and metadata/count readback remain required.
Only then, after current identity and validity revalidation, is a result recorded as applied.
A result that expires during ACK/readback fails the worker closed and enters its
existing owned drain; successful transport cannot make that expired result applied.
Evaluation diagnostics optionally include an observed numeric model_http_status;
absence means no HTTP status was observed, never a status inferred from policy.
The evaluation diagnostic stays immutable with publication=not_attempted.
Separate decision receipts and publication IDs record later publication.

## Minimum private storage and restart semantics

The existing version-1 lifecycle journal remains. Beside it,
state-file.decisions.jsonl is a version-1 append-only event ledger. Events retain
random session/decision IDs, triggering connection ID, private target/evidence
metadata, suggestion separately from adoption, validity, policy/scope versions,
status and publication ID. It contains no API key, controller token, node
credential or raw model response. Keep its parent directory private; portable
file modes do not claim to establish Windows ACLs. Never upload the ledger.

Every event is synced before trust. A process holds at most 4096 records.
A nonregular or >=16 MiB ledger blocks further recording. There is no database,
automatic rotation, platform service or cross-session monetary budget.
Archive private evidence separately and keep an approved provider account cap.

Each new process has a new session and empty decision memory. It never reloads
ledger events as rules or authority. Existing startup empty reconciliation still
precedes readiness. Successful stop/maintenance empty readback marks pending and
applied records withdrawn. Expired records become expired. A crash can leave
cached rules until verified independent empty recovery; expiry does not run
inside a stopped core. Recovery still requires confirmed publisher exit and
unchanged ownership, and never restarts learning. New profile/core generations
require new ownership.

Status adds host-free decisions and publication_schedule: interval, first due
time, first nonempty PUT-attempt time and nonempty PUT-attempt count. Failed
transports count as attempts; these fields do not prove ACK/application.
Correlate decision and publication IDs to actual connection IDs, target,
rule payload and chain in same-run private evidence before publishing a redacted
report.

Focused offline tests cover pre-tick zero submission, a fresh publication context
after judgment cancellation, no-op ticks, expiry/rejection/late/version refusal,
current ownership loss, durable correlation/restart non-restoration, and failed
ACK. Existing policy, fresh-fetch, maintenance and recovery guards remain.
All execution is on GitHub-hosted Windows. No real learning, current nonempty
FlClash integration or TUN acceptance is claimed by offline preparation.
