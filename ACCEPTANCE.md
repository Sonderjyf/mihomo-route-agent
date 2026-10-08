# Remaining usable-outcome acceptance

Code/package delivery is complete for the experimental milestone; usable production routing is **not accepted**. The real collector is shadow-only. Passing unit tests or SOCKS smoke does not close the actual Windows TUN, FlClash application or independent-network gates.

| Gate | Current evidence | Still required |
|---|---|---|
| Build and policy boundaries | Windows/Linux Go checks; local verified TLS and model-wire tests | Continue regression checks |
| Original-rule priority and async fallback | Separate Mihomo SOCKS/Fake-IP smoke, TCP 443 scope, TCP 8443 fallback | Same observations from real native TUN connections |
| Recovery | TTL, restart eviction, graceful cleanup; crash-cache limitation reproduced | Core/app restart, settings refresh, profile switching, supervised crash handling |
| Real TLS facts | Local DNS/TLS/CONNECT and certificate verification | Independent approved direct/proxy paths and evidence freshness under real conditions |
| FlClash integration | Source inspection and offline profile plans | Actual app-generated config/runtime agreement, refresh behavior and reversible application |
| Real learned routes | Deliberately unavailable; shadow only | Verified ownership, fresh evidence and controlled publication acceptance before enabling |

## Minimum next execution: disposable Windows core TUN

The new `scripts/tun_acceptance.py` defaults to an inert JSON plan. Running it without flags never queries host networking, binds sockets or starts a process. Actual execution requires `--execute --allow-isolated-tun`, an administrator session and the expected GitHub-hosted Windows runner/repository identifiers. The environment checks prevent accidental host/self-hosted execution; they are not a cryptographic attestation. Do not forge them to run on a personal computer.

The prepared `.github/workflows/owned-windows-tun.yml` runs only when the exact label `accept-core-tun-on-hosted-windows` is added to this repository's PR #5 from `codex/review-dns-correctness`. Publishing this workflow, a normal push or ordinary PR CI does not grant permission or launch the test. Add that label only after explicit approval of the following operation:

- One disposable `windows-2025` standard hosted VM; 20-minute job timeout. Estimated execution 5–15 minutes, not a measured result.
- Download Go/dependencies and the official Mihomo v1.19.32 Windows ZIP into the guest, checking the pinned archive SHA256. No host installation or Windows VM image is needed.
- Create an owned guest Wintun adapter and route only `198.19.0.0/16` to it. System proxy is untouched; no firewall change, default-route replacement, real subscription, API key or paid model is used.
- Resolve synthetic `.test` fixtures against a guest loopback DNS service, then connect ordinary native sockets to the Fake-IP address. Require the Windows route to point to the owned adapter and actual core metadata to report `TUN`, with expected MATCH/AND chains and original-rule priority.
- Repeat the observer's crash/restart/TTL/graceful-cleanup tests; compare guest default routes, existing-adapter DNS and owned-prefix routes before/after. Unexpected changes or failed cleanup fail the job. Terminate only recorded child PIDs; the platform discards the guest afterward.
- Emit only synthetic JSON results to workflow logs. No artifacts/caches/custom images or GitHub Release are created by this acceptance job.

GitHub documents standard public-repository Windows runners as fresh VMs with 4 CPUs, 16 GB RAM, 14 GB SSD, administrator rights and free standard-runner usage. This repository was verified public on 2026-10-08. The proposed job uses no larger runner and disables setup-go caching. Sources: [runner specifications](https://docs.github.com/en/actions/reference/runners/github-hosted-runners), [billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions). No paid cloud account is requested.

This is a concrete **core TUN** test, not a claimed FlClash UI test or proof of a useful external direct/proxy distinction. Even successful execution leaves the application/settings and real-evidence gates open. Follow-up FlClash testing needs its own controlled guest setup and source/version-matched app automation; official portable release availability alone does not establish that its GUI or privileged helper works unattended.

## Host inventory outcome and route independence

Read-only inventory found an existing WSL2 registration but no usable Windows guest or common desktop VM executable in checked locations. WSL enumeration, Hyper-V inventory and hardware CIM access were denied; no elevation or alternate access to those protected objects was attempted. The inventory is partial, not proof that no VM exists anywhere. No unrelated guest was booted or changed.

The active host retains a preferred TUN default route. NAT/WSL traffic can inherit that path, so a new NAT guest alone would not establish an independent direct route. Do not disable host TUN, change adapters, bridge the host network or install virtualization drivers to make this test pass. A separately owned Windows machine or the disposable hosted guest can isolate host impact; actual external-path independence still requires direct evidence from that test environment.

## Safe checks before permission

```powershell
python -B scripts/tun_acceptance.py
python -B -m unittest discover -s scripts -p test_tun_acceptance.py -v
```

The ordinary `smoke_observe.py` command continues to use loopback SOCKS with TUN disabled. Run it only with a separate supplied core and new work directory as documented in OBSERVATION.md. Prepared TUN code and guard tests are not recorded as TUN acceptance passed until the guest job actually runs and its observations pass.
