# Lab lifecycle: journal, eviction and owned cleanup

`observe-lab` now requires `--state-file PATH` in an existing private directory. This is a durable lifecycle record, **not a restored learned-rule cache**. It records a format version, a fingerprint of controller/listener/rule metadata, phase, learned entries and absolute expiry times. Writes use a temporary file, file sync and replacement; filesystem/power-loss atomicity is not promised. No journal is published as evidence.

On every start, including when the state file is missing, all old learned entries are treated as untrusted. After validating the current tail rules, the observer writes `reconciling`, force-reloads two empty providers, checks fresh payload fetches and provider metadata/counts, rechecks the core rules, and only then records `active` and reports ready. It never restores old journal entries. Expired and unexpired historical entries are both evicted; later eligible connections may be learned again. A corrupt, unsupported-version or differently owned existing journal stops startup without overwriting that record.

The running observer records mutation intent before changing providers and the resulting active snapshot after success. TTL pruning updates both core providers and the journal. Provider failures stop the observer; it attempts owned cleanup before closing its HTTP service. A graceful context cancellation or bounded `--run-for` expiry also attempts cleanup. Cleanup requires the pinned rule configuration to match before and after reloading empty providers. Success records `stopped`; failed/uncertain ownership or incomplete cleanup records `recovery_required` when the journal remains writable and exits unsuccessfully. `--run-for` is a lab test mechanism, not a service scheduler.

## What provider identity means here

The observer additionally requires each managed provider to report its expected name, `Classical` behavior and `HTTP` vehicle. A successful reload must cause a fresh completed GET of the corresponding payload from this observer during that PUT, followed by the expected rule count. A fake/wrong source returning the same count without fetching this observer is rejected. Other Agent modes keep their earlier behavior; this stricter check is scoped to the lab observer.

This is a stronger **exclusive-lab consistency check**, not cryptographic ownership or an API-provided content digest. Provider metadata does not expose a binding to the configured source URL, and fetches are not correlated with a core-issued transaction ID. Concurrent or spoofed local requests can invalidate the inference. A fingerprint is an accidental-reuse guard, not authentication. Production ownership still requires an integration contract and stronger core identity/content evidence.

## Reproduce and verified results

~~~powershell
go build -trimpath -o dist/route-agent.exe ./cmd/route-agent
python scripts/smoke_observe.py --agent dist/route-agent.exe --mihomo C:/tools/mihomo.exe --workdir local-evidence/lifecycle-new-run --lifecycle
~~~

The harness supplies a private state file automatically and creates only synthetic services/configuration in a new directory. It protects laboratory ports before launch and accepts an optional private `--protected-ports` JSON list. It does not touch installed FlClash, system settings, real credentials or user traffic.

Verified against official Mihomo v1.19.32 on Windows:

- Abruptly killing the observer leaves the already learned rule in the still-running core. This explicitly demonstrates the crash-cleanup limit.
- Restarting against that core evicts the cached rule before ready; the old journal entry is not restored as trusted.
- A newly learned entry expires while the observer runs; both the core count and observer count return to zero.
- A separate graceful exit with an unexpired entry clears the owned providers and leaves a `stopped`, empty journal.
- Mode drift stops the observer and records `recovery_required`; cleanup does not mutate a core whose rule-mode identity no longer matches.
- Go tests cover persisted TTL, journal replacement/corruption/owner mismatch, failure to fetch the observer despite matching counts, and rule drift during provider PUT.

Results are in `evidence/lifecycle-core-2026-10-07.json` and `evidence/lifecycle-drift-2026-10-07.json`.

## Remaining limits

An abrupt kill, machine outage or core disconnection prevents cleanup. During downtime, and before the next startup reconciliation completes, a still-running core can route new connections using stale cached learned rules. Expiry is not enforced inside the core while the observer is absent. No Windows service, autostart or supervisor has been installed. This milestone prevents startup from silently trusting stale state; it does not eliminate that routing window.

Rule checks now bracket provider commits and TTL updates, and a detected change prevents recording a trusted successful commit. There is still no atomic configuration generation shared between `/rules`, `/connections` and provider PUTs. An A→B→A change or a reload after the last check can evade detection. Detection also cannot undo traffic already routed during a race. On uncertain ownership, stop and record recovery-required rather than rewriting original rules. This limitation must be resolved or explicitly bounded before live acceptance.

Original rule order remains untouched. Restoring learned entries across restart, crash-time expiry, journal retention/rotation, transactional multi-provider updates and authenticated source binding remain future work; real evidence/model and actual FlClash/TUN acceptance are still separate.
