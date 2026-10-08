# Remaining usable-outcome acceptance

Code/package delivery is complete for the experimental milestone; usable production routing is **not accepted**. The default real collector is shadow-only; a separate opt-in controlled publisher is now implemented with local synthetic tests and remains live-unaccepted. Passing unit tests or SOCKS smoke does not close the actual Windows TUN, FlClash application or independent-network gates.

The independently authorized hosted Windows **core TUN** gate passed on 2026-10-08 at tested head `bf8fc3945736687646e16ff40b26083269172ac7`: [run 37722276208, attempt 1](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37722276208). This result is narrower than complete FlClash/product acceptance.

| Gate | Current evidence | Still required |
|---|---|---|
| Build and policy boundaries | Windows/Linux Go checks; local verified TLS and model-wire tests | Continue regression checks |
| Original-rule priority and async fallback | Separate Mihomo SOCKS and actual hosted Windows TUN: first fallback, later TCP 443 learning, TCP 8443 fallback | Equivalent behavior with actual FlClash-generated configuration |
| Recovery | SOCKS and hosted TUN TTL/restart/graceful cleanup; crash-cache limitation reproduced | Core/app restart, settings refresh, profile switching, supervised crash handling |
| Real TLS facts | Local DNS/TLS/CONNECT and certificate verification | Independent approved direct/proxy paths and evidence freshness under real conditions |
| FlClash integration | Source inspection and offline profile plans | Actual app-generated config/runtime agreement, refresh behavior and reversible application |
| Real learned routes | Opt-in publisher implemented; local TLS/ownership/pause tests pass | Actual Windows path checks, app-coordinated refresh, external evidence and production supervision acceptance |

## Next application gate

The first approved actual FlClash app attempt [37727215535](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37727215535) failed in 45 seconds at tested SHA `b93f5e3fb252729f2b469a6dca31b05e571e0ab0`. The verified official app launched, but the first GUI action found zero matching invokable `Profiles` controls. Configuration refresh and normal exit were not tested. Recorded-PID forced cleanup and the final unchanged guest-network snapshot were reported. See `evidence/windows-flclash-app-attempt-2026-10-08.json` and FLCLASH_ACCEPTANCE.md. The trigger label is absent and this single-run approval is consumed.

The next proposed gate is actual FlClash configuration generation, profile refresh and start/exit with TUN **disabled**. See [FLCLASH_ACCEPTANCE.md](FLCLASH_ACCEPTANCE.md) for pinned release/source findings, the offline contract checker, current GUI-automation blockers and a bounded guest permission proposal. No application run or new TUN run has been authorized by that preparation. Controlled real-evidence publication is now implemented behind explicit runtime guards; live acceptance and production supervision remain open.

## Completed separately authorized execution: disposable Windows core TUN

The new `scripts/tun_acceptance.py` defaults to an inert JSON plan. Running it without flags never queries host networking, binds sockets or starts a process. Actual execution requires `--execute --allow-isolated-tun`, an administrator session and the expected GitHub-hosted Windows runner/repository identifiers. The environment checks prevent accidental host/self-hosted execution; they are not a cryptographic attestation. Do not forge them to run on a personal computer.

The prepared `.github/workflows/owned-windows-tun.yml` runs only when the exact label `accept-core-tun-on-hosted-windows` is added to this repository's PR #5 from `codex/review-dns-correctness`. Publishing this workflow, a normal push or ordinary PR CI does not grant permission or launch the test. Add that label only after explicit approval of the following operation:

- One disposable `windows-2025` standard hosted VM; 20-minute job timeout. Estimated execution 5–15 minutes, not a measured result.
- Download Go/dependencies and the official Mihomo v1.19.32 Windows ZIP into the guest, checking the pinned archive SHA256. No host installation or Windows VM image is needed.
- Create an owned guest Wintun adapter and route only `198.19.0.0/16` to it. System proxy is untouched; no firewall change, default-route replacement, real subscription, API key or paid model is used.
- Resolve synthetic `.test` fixtures against a guest loopback DNS service, then connect ordinary native sockets to the Fake-IP address. Require the Windows route to point to the owned adapter and actual core metadata to report `Tun`, with expected MATCH/AND chains and original-rule priority.
- Repeat the observer's crash/restart/TTL/graceful-cleanup tests; compare guest default routes, existing-adapter DNS and owned-prefix routes before/after. Unexpected changes or failed cleanup fail the job. Terminate only recorded child PIDs; the platform discards the guest afterward.
- Emit only synthetic JSON results to workflow logs. No artifacts/caches/custom images or GitHub Release are created by this acceptance job.

GitHub documents standard public-repository Windows runners as fresh VMs with 4 CPUs, 16 GB RAM, 14 GB SSD, administrator rights and free standard-runner usage. This repository was verified public on 2026-10-08. The proposed job uses no larger runner and disables setup-go caching. Sources: [runner specifications](https://docs.github.com/en/actions/reference/runners/github-hosted-runners), [billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions). No paid cloud account is requested.

This is a concrete **core TUN** test, not a claimed FlClash UI test or proof of a useful external direct/proxy distinction. Even successful execution leaves the application/settings and real-evidence gates open. Follow-up FlClash testing needs its own controlled guest setup and source/version-matched app automation; official portable release availability alone does not establish that its GUI or privileged helper works unattended.

## Host inventory outcome and route independence

Read-only inventory found an existing WSL2 registration but no usable Windows guest or common desktop VM executable in checked locations. WSL enumeration, Hyper-V inventory and hardware CIM access were denied; no elevation or alternate access to those protected objects was attempted. The inventory is partial, not proof that no VM exists anywhere. No unrelated guest was booted or changed.

The active host retains a preferred TUN default route. NAT/WSL traffic can inherit that path, so a new NAT guest alone would not establish an independent direct route. Do not disable host TUN, change adapters, bridge the host network or install virtualization drivers to make this test pass. A separately owned Windows machine or the disposable hosted guest can isolate host impact; actual external-path independence still requires direct evidence from that test environment.

## Safe checks before permission

The corrected, separately approved run `37722276208` completed successfully in 80 seconds. Logs contain real core `inbound_type: Tun` records: the first native connection stays MATCH/BASE, later TCP 443 traffic uses AND/LEARNED, TCP 8443 stays MATCH/BASE, original domain rules take precedence, and uncertain evidence is not learned. Mode drift stops the observer. The second phase verifies crash-cache residue, restart eviction, TTL removal and graceful cleanup. Both phases verify restoration of guest default routes, existing-adapter DNS, the owned interface and prefix routes. The trigger label was removed after admission; no further network run was started.

Verified synthetic reports are `evidence/windows-tun-mode-2026-10-08.json` and `evidence/windows-tun-lifecycle-2026-10-08.json`. They identify the tested commit and run, independent of later documentation commits. This validates actual native TUN transport with synthetic reachability evidence; it does not validate real TLS/network quality, paid model decisions, FlClash UI/settings or production ownership/supervision.

The first explicitly approved hosted run, [37721526537](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37721526537), failed on 2026-10-08. Guest setup, pinned archive verification, execution guards, owned-prefix route checks and native socket echo completed; the harness then incorrectly required inbound type `TUN` while pinned Mihomo v1.19.32 serializes that enum as `Tun`. The old report did not retain the observed type, so it remains a failed/incomplete acceptance result, not a retroactive pass. Guest route/DNS/interface restoration passed; the later lifecycle step was skipped. See `evidence/windows-tun-attempt-2026-10-08.json`.

The exact-case predicate is now fixed and covered by an offline test; reports retain observed inbound types before validating them. A user-initiated retry of the old run also used the old commit and failed at the same assertion, with cleanup verified. The corrected head was then tested only after renewed explicit approval. Both single-run approvals have been consumed and the trigger label is absent. No automatic further network run is authorized.

```powershell
python -B scripts/tun_acceptance.py
python -B -m unittest discover -s scripts -p test_tun_acceptance.py -v
```

The ordinary `smoke_observe.py` command continues to use loopback SOCKS with TUN disabled. Run it only with a separate supplied core and new work directory as documented in OBSERVATION.md. Prepared TUN code and guard tests are not recorded as TUN acceptance passed until the guest job actually runs and its observations pass.
