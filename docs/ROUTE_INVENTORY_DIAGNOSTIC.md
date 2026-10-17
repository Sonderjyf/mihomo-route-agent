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

The investigation used ordinary secret-free Windows CI, identical inventory/physical-guard code and fixed environment comparisons. It read local route/adapter tables, sent no target packets, started no core, and received no node or API credentials. Only validated typed diagnostics were printed. The initial comparisons ran the full environment first, so shared provider warm-up remained a limitation.

This work does not change main or authorize another live run.

## First hosted comparison

Run `37878650300` at `5db1fc4d2585e8b43587029848e528d6e1d619ab` reproduced the failure without core or secrets: normal runner inventory passed in 1626 ms and physical guard in 2220 ms; the actual minimal worker environment timed out at 15021 ms; adding runtime/profile/module paths still timed out at 15017 ms. Both timeouts had no stderr. Linux CI passed; the Windows comparison failed and later Windows steps were skipped.

This establishes an environment-sensitive reproduction, not the precise missing dependency or proof of the old live run's unreported subreason. The second comparison tested machine/identity variables in both directions. No environment values were emitted.

The second comparison (`37879045004`, `dff12d9132ad19fca07770dadad9725e7effae06`) ruled out COMPUTERNAME as the sufficient/necessary difference: adding it to minimal still timed out; baseline without it passed (630 ms, physical guard 1356 ms). The identity group and combined runtime/identity group also timed out. Baseline passed at 1191 ms, physical guard 1800 ms. All minimal-derived timeouts were about 15013-15015 ms with no stderr.

## Isolated module-cache dependency and minimal fix

Third comparison: run `37879640653`, SHA `7b92cab7779026b8318fecd23b22e825ff59c73f`.

| Environment | Inventory | Original physical guard |
| --- | --- | --- |
| Full runner | Passed, 1379 ms | Passed, 1856 ms |
| Original worker allowlist | Timeout, 15020 ms | Not run |
| Original allowlist plus only `PSModuleAnalysisCachePath` | Passed, 741 ms | Passed, 1358 ms |
| Full runner minus only that variable | Timeout, 15017 ms | Not run |

A separate test-only marker script completed import/query in 656 ms under the baseline. Under the original minimal environment it timed out after 15031 ms, with `import_started` as its last marker. Thus the reproduced stall is inside `Import-Module NetTCPIP`, before the route query. Process execution-policy and lockdown-hook variables were absent. No environment values were printed.

The add/remove controls establish the module-cache variable as the decisive environment difference for this hosted reproduction. They do not establish why the default cache stalls internally, prove behavior on every runner, or retroactively recover the missing subreason from live run `37877655480`.

The fix adds only `PSMODULEANALYSISCACHEPATH` to `child_env`'s case-insensitive allowlist, preserving the parent's original value when present and inventing no value when absent. It does not parse, execute, print, create or clean up that path. PowerShell itself retains its normal cache behavior. Execution policy, module search paths, lockdown hooks and unrelated credentials remain excluded. The original inventory script/15-second budget and physical guard/3-second budget are unchanged, with the same fail-closed checks.

[Windows PowerShell 5.1 documentation](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_windows_powershell_5.1?view=powershell-5.1) describes this variable as the module-analysis cache location, including disabling file caching with a device path. Preserving the runner's setting avoids introducing an untested cache location or changing security policy.

## Ongoing verification

The hosted regression requires the actual fixed worker environment to pass inventory and the unchanged physical guard both before and after its comparisons. The full-environment baseline must also pass. The legacy environment without the cache variable remains an observation: its typed timeout does not fail the job, and future runner improvements may legitimately make it pass. Malformed output or an unavailable child still fails the comparison. No positive-case failure is ignored or retried.

The fixed worker runs before this comparison's baseline; earlier ordinary tests may still have warmed OS/module state, so this is not a pristine-machine cold-start proof. Secret-free comparison processes contain no model/node/controller credentials and no real acceptance is executed.

Offline tests cover all 16 inventory success/refusal cases with redaction canaries, strict nested output validation, case-insensitive cache inheritance with spaces, absent-variable behavior, credential/policy exclusion, and the regression driver's positive/negative/fail-closed logic. Actual TUN, real API calls, real proxy learning and routing publication remain untested by this investigation.
