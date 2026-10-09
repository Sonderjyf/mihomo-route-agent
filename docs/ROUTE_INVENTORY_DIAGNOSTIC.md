# Route inventory failure investigation

Observed live run: `37877655480`, execution SHA `6870f4de61d8f2b9ffb2fe21b3f94a8516a4b71e`. The earlier physical-route prerequisite passed; the later worker returned `route_inventory / guard_rejected`, zero API attempts and verified cleanup. Log masking damaged JSON punctuation, so the archived observations are readable allowlisted fields, not a revalidated complete JSON record. The old worker did not expose a query subreason; elapsed step time alone does not establish timeout.

## Actual refusal branches

The unchanged inventory query uses a 15-second budget and requires exactly two `Find-NetRoute` rows before returning the first interface index. The production physical-path check still independently uses three seconds and verifies the selected physical adapter and both route rows.

The worker previously collapsed all these cases into `guard_rejected`:

- Query could not start, was canceled, timed out, failed I/O, exceeded the output limit, or exited unsuccessfully.
- Explicit exit 7 from the inventory script means its route-row count check rejected the result; other exit codes remain generic query failures.
- Successful stdout could still fail integer conversion, including empty/extra output or overflow.
- Parsed interface index could be zero or negative.

The shared `acceptanceRouteInventory` helper preserves the original script and budget. Its optional `route_inventory` result reports only fixed reasons, elapsed/budget milliseconds, a known process exit code or null, and stderr presence or null. It never serializes addresses, raw output, exception messages, script text or environment values. Normal query success does not prove stderr was absent, so that value stays null. The outer failure remains fail-closed.

## Environment and ordering differences

The preflight runs with the normal runner environment and inventories physical adapters before `Find-NetRoute`. The worker receives the Python allowlist of Windows root/temp/path variables, then executes `Find-NetRoute` directly. Its environment omits user/profile/module/runtime locations. These differences are hypotheses, not a demonstrated root cause.

A local secret-free `Get-Command Find-NetRoute` check succeeded both with the original worker allowlist (925 ms) and an expanded list of Windows system locations (848 ms). It did not query routes and does not reproduce the hosted failure.

Ordinary Windows CI now compares the identical inventory helper and subsequent unchanged physical guard in three child processes: normal runner environment, the actual Python worker allowlist, and that allowlist plus selected Windows runtime locations. This reads only local route/adapter tables, sends no target packets, starts no core, and receives no node or API credentials. It prints only validated typed diagnostics. A failed case fails the comparison; it does not trigger retries or bypass a production guard.

The baseline runs first, like the original preflight-before-worker sequence. Shared CIM/provider warm-up and time-dependent runner state can affect subsequent cases; a later minimal-environment pass cannot rule out an earlier cold-start problem or prove environment equivalence.

No runtime environment expansion or timeout increase is justified before comparison evidence. This diagnostic revision does not change main or authorize another live run.

## First hosted comparison

Run `37878650300` at `5db1fc4d2585e8b43587029848e528d6e1d619ab` reproduced the failure without core or secrets: normal runner inventory passed in 1626 ms and physical guard in 2220 ms; the actual minimal worker environment timed out at 15021 ms; adding runtime/profile/module paths still timed out at 15017 ms. Both timeouts had no stderr. Linux CI passed; the Windows comparison failed and later Windows steps were skipped.

This establishes an environment-sensitive reproduction, not the precise missing dependency or proof of the old live run's unreported subreason. The next comparison adds fixed machine/identity variables, tests COMPUTERNAME alone, and removes COMPUTERNAME from the baseline to check both directions. No environment values are emitted or copied into production yet.

The second comparison (`37879045004`, `dff12d9132ad19fca07770dadad9725e7effae06`) ruled out COMPUTERNAME as the sufficient/necessary difference: adding it to minimal still timed out; baseline without it passed (630 ms, physical guard 1356 ms). The identity group and combined runtime/identity group also timed out. Baseline passed at 1191 ms, physical guard 1800 ms. All minimal-derived timeouts were about 15013–15015 ms with no stderr.

The third comparison checks only the presence and add/remove behavior of three fixed PowerShell variables, and uses a test-only marker script to separate module import from CIM query under one 15-second total budget. These are diagnostic cases, not production policy changes. No policy value is printed, and neither process execution policy nor lockdown hooks will be propagated into production merely to make a test pass.
