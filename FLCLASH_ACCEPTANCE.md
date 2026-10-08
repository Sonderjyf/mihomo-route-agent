# FlClash application acceptance: prepared, not executed

The next smallest useful gate is actual FlClash configuration generation and refresh with **TUN off**, followed by app/core start and graceful exit. The successful standalone Mihomo TUN run does not close this gate. No FlClash binary was downloaded, installed or launched for this preparation; no guest job was started.

## Pinned application and source findings

Read-only release metadata on 2026-10-08 identifies [official v0.8.99](https://github.com/chen08209/FlClash/releases/tag/v0.8.99), source commit `68c71b8ef9b7486a224972eb371ff153c6b2de0f`, and `FlClash-0.8.99-windows-amd64.zip` (67,644,330 bytes). The published asset digest is SHA256 `9ff3a9315b51e6665669bfde9b0323ddb671a22d0a9647987b4cc5e574007f4f`. It has not been verified against a downloaded archive here. The local research checkout is a different revision; the findings below use the release commit's files.

| Finding | Consequence | Pinned source |
|---|---|---|
| `getProfile` evaluates the script before `makeRealProfileTask` | Script output is intermediate, not effective config | [setup.dart:355](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/providers/actions/setup.dart#L355) |
| The app replaces controller, ports, TUN and other settings; non-inline provider paths are confined under the profile's cache directory | Check the generated config and API; never assume a preview's relative provider path survives | [task.dart:130](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/common/task.dart#L130) |
| DNS overrides and `appendSystemDns` apply after the script | Disable both for this fixture and compare complete DNS maps | [task.dart:247](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/common/task.dart#L247) |
| The enabled controller enum is `127.0.0.1:9090` | Existing standalone smoke's controller at 19091 is not the app endpoint | [enum.dart:341](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/enum/enum.dart#L341) |
| Active URL-profile refresh calls `applyProfileDebounce`; setup writes `config.yaml` and calls the app-owned core | Editing generated YAML once is not a refresh integration | [profiles.dart:99](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/providers/actions/profiles.dart#L99), [setup.dart:595](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/providers/actions/setup.dart#L595) |
| `NetworkProps.systemProxy` and `autoSetSystemDns` default true | Explicitly seed/verify both false before starting the guest core | [config.dart](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/models/config.dart) |
| SAFE_MODE is a compile-time constant; it closes the controller. App data is in application-support storage. Normal Windows startup registers URL schemes | An extracted ZIP is not a no-side-effect sandbox; `--safe-mode` is not a supported runtime protection | [constant.dart:42](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/common/constant.dart#L42), [setup.dart:482](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/providers/actions/setup.dart#L482), [path.dart](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/common/path.dart), [window.dart](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/lib/common/window.dart) |

These are source findings, not reproduced release-application results. Upstream's build workflow contains Windows build checks; that does not establish unattended GUI interaction on our hosted runner. Release accessibility/UI automation and preference seeding still need a bounded guest feasibility step. Do not silently substitute an instrumented/source build for the official release.

## Safe result available now

`scripts/flclash_acceptance.py` is an offline checker, with no process launch, sockets, network inspection, installation or execution mode. With no arguments it prints the pinned plan. With three arguments it reads two JSON configuration snapshots and checks the narrow fixture contract:

```powershell
python -B scripts/flclash_acceptance.py
python -B -m unittest discover -s scripts -p test_flclash_acceptance.py -v
python -B scripts/flclash_acceptance.py --original-json original.json --effective-json effective.json --proxy-target LAB
```

The original must be the untouched synthetic profile from that refresh revision. The effective snapshot must later be captured from the app-generated YAML and losslessly parsed to JSON. The checker requires exact original-rule order, exactly two TCP 443 managed rules before the terminal MATCH, unchanged DNS/proxies/groups, loopback controller 9090, TUN and LAN disabled, original providers preserved except their app-owned paths, and managed HTTP providers at `127.0.0.1:18765` with explicit `proxy: DIRECT`. For this fixture use the same provider definitions as `smoke_observe.py`, not `tail-preview`'s disconnected local-file placeholders.

Passing the checker says only `config_contract_passed: true`; it always says `app_tested: false` and `tun_tested: false`. It does not prove snapshot provenance, app state, provider file existence, runtime behavior or cleanup. Tests use invented in-memory fixtures and must never be published as observed application evidence.

## Bounded next execution proposal

Prepare a separate one-shot, manually approved guest workflow; do not reuse/add the core-TUN label. Pin the repository SHA before approval. Its maximum scope is one standard public-repository `windows-2025` guest for 20 minutes, with no automatic retry, remote cache, uploaded artifact, installer, service installation, TUN, firewall change or external target/model request.

1. Download only the pinned official portable app/dependencies into the guest, verify the archive digest, record bundled core version/hash, and fail on mismatch. Baseline guest WinINET/WinHTTP proxy, default routes, adapter DNS and app/helper processes. Use fresh guest app data, never copied personal settings/subscriptions.
2. Before starting the core, set and read back system proxy, automatic system DNS, TUN, LAN access, startup/autostart, automatic updates, profile auto-update, telemetry and background network detection as disabled. Read back settings through the app or its persisted settings. An unreadable/unsupported setting is a blocker, not a reason to launch with defaults. Guest application-support files and URL-scheme registration are allowed effects of this proposed app run; host effects remain prohibited.
3. Launch only the recorded app binary/PID in the guest. Establish a usable UI automation session first. If the desktop/Flutter controls are inaccessible, record `blocked_gui_automation`, clean up and stop the same job. Do not install a remote desktop agent, invoke an unapproved service, or rerun automatically.
4. Through the actual app, import a guest-loopback URL profile and bind a synthetic JavaScript overwrite that inserts only the two empty HTTP providers/tail rules. Serve all fixtures on guest loopback, keep TUN off, and use explicit localhost SOCKS connections. Capture generated YAML plus actual `/configs`, `/rules`, `/providers/rules` and fixture connection records; distinguish generated-config checks from runtime observations. Initial providers must be empty so this gate does not claim real learning.
5. Change the loopback subscription from revision A to B by adding an explicit `.test` rule. Invoke refresh through the app, not a direct core reload or disk replacement. Require revision B's original rules, exactly one managed tail, preserved DNS/provider URLs, and runtime agreement. Repeat refresh of B to detect duplicate insertion. A script-only replay is not app-refresh evidence.
6. Stop/start the core through the app and verify profile B remains effective. Exit through the app, confirm owned app/core PIDs and listeners terminate, and compare guest proxy, routes and DNS to baseline. Force-stopping recorded children is emergency cleanup, not a graceful-exit pass. No process-name-wide termination. The VM is then discarded.

No executable guest/UI collector or workflow for this proposal is implemented yet. The offline checker is ready; exact release preference serialization and GUI selectors are the immediate implementation requirements. Approval must cover the bounded feasibility attempt, not promise all six steps can run unattended. A fresh TUN run is unnecessary for this first app gate and remains unauthorized.

Suggested bundled approval, once the guest collector/workflow is reviewable:

> Allow exactly one disposable GitHub-hosted Windows FlClash v0.8.99 application feasibility/acceptance run, pinned to the reviewed route-agent SHA, maximum 20 minutes. Download and checksum-verify the official portable ZIP; permit guest-only application data and URL-scheme registration. Keep TUN, system proxy, automatic DNS and startup disabled, use synthetic localhost fixtures only, and test app-generated config, profile refresh and start/exit if GUI automation works. Stop on an automation/settings blocker or unexpected network-setting change, clean up recorded child processes, and discard the guest. No host changes, service install, real subscription, credentials, external model/target requests, paid resources, artifacts/caches, merge/release or automatic retry.

## Remaining product decision

`probe` reports `routing_updated: false`; `observe` constructs `NewShadowObserver`, which has no publisher. Only synthetic `observe-lab` publishes learned routes. Therefore actual real-evidence learning is a missing production feature, not merely a pending acceptance test. Passing this app gate will not implement it.

The smallest later feature requires an explicit product scope: either ship an advisory/shadow tool, or implement an opt-in real-evidence publisher. The latter needs app/profile ownership and refresh handoff, verified independent direct/proxy paths, fresh evidence, bounded publication/revocation, recovery/supervision for stale cached rules, and an independently approved model trial if desired. Current snapshots lack an atomic app/core configuration epoch; source review does not remove that race. Do not enable the lab publisher for real hosts or broaden the allowlist as a shortcut.
