# Experimental router delivery

This package completes the bounded offline/laboratory engineering milestone: offline profile assessment and tail planning; explicit real TLS collection and read-only shadow observation; synthetic asynchronous learning in an owned core; lifecycle recovery; tests and a Windows package. It is not a production FlClash installer. Current command behavior below supersedes older milestone descriptions.

## Start offline

Run from the extracted package directory. These commands do not bind ports, read credentials or make network requests:

```powershell
./route-agent.exe version
./route-agent.exe check --config config.example.json
./route-agent.exe check --config examples/observation.shadow.json --allow-lab-fixtures
./route-agent.exe assess --profile examples/isolated-profile.yaml --output new-assessment.json
./route-agent.exe tail-preview --profile examples/isolated-profile.yaml --proxy-target PROXY --output new-tail-plan.json
```

Only new output files are created. A tail plan preserves opaque original fields and the complete original rule order. It inserts two managed `AND` rules immediately before a unique terminal MATCH, each requiring `NETWORK,tcp`, `DST-PORT,443` and its DOMAIN rule set. UDP/QUIC and other destination ports retain the original policy. Plans contain original profile credentials: keep personal plans private. The bundled profile is synthetic. Do not apply a personal plan automatically.

## Real evidence: read-only commands

`examples/observation.shadow.json` is an inert configuration example, with lab-port placeholders. `check` validates syntax, not connectivity or route independence. Configure your own explicitly authorized DNS endpoint, HTTP CONNECT proxy, controller, unused status port and exact host allowlist in a separate private copy before running:

```powershell
./route-agent.exe probe --config config.local.shadow.json --allow-lab-fixtures --allow-external-probes example.com
./route-agent.exe observe --config config.local.shadow.json --allow-lab-fixtures --allow-external-probes --run-for 30s
```

These commands perform network activity only with `--allow-external-probes`. The example uses the stub judge, so `--allow-lab-fixtures` is also required; the stub always returns UNCERTAIN. `probe` accepts one exact allowlisted hostname and prints coarse collected evidence plus a candidate decision. `observe` reads the configured controller, collects evidence only for allowlisted fresh TCP 443 terminal-MATCH connections, and serves aggregate `/status` counters. It has no provider endpoint, provider writes or journal. The actual collector is wired to both commands; neither can apply a real-evidence decision.

Each observation run attempts each host at most once and no more than `max_api_requests` hosts. Each eligible attempt can make at most one model request; collection/model work has a 10-second maximum and respects shorter cancellation/deadlines. `--run-for` bounds observer lifetime to up to ten minutes when specified; zero is foreground operation until canceled. Failed or missed snapshots are not retried per host. Changing core mode/rule metadata stops observation. Counts reset each process; no cross-restart monetary budget is implemented.

To use Jev, choose `judge: "jev"`, configure any required explicit `api_proxy`, and separately supply `--allow-model-api`. Only after that flag is checked does the command read `OPENROUTER_API_KEY`; never put its value in a configuration, command argument, report or repository. `MIHOMO_SECRET` is optional controller authentication for shadow reads. Model state contains hostname, registrable name and coarse TLS evidence labels. The model endpoint is OpenRouter Decisions / `typesafe/jev-1.13`. A request count is not a hard dollar cap; account controls and separate approval are required for a paid trial. No model call is necessary for local tests or package checks.

TLS uses verified SNI/certificates, pins a numeric DNS answer across direct/CONNECT attempts, rejects protected/Fake-IP answers, and sends no application request to target websites. See [PROBES.md](PROBES.md). A normal socket can still traverse an active TUN: `--allow-external-probes` does not certify a physical direct path. The current machine's preflight could not establish independent paths, so external validation stopped before target DNS/TLS/model requests. Controlled publication is available only through the separately guarded command below and remains live-unaccepted.

## Controlled publication and refresh handoff

`observe-apply` connects the real collector to the bounded TCP 443 publisher. It requires `--allow-controlled-apply`, `--exclusive-controller`, a private `--ownership` file, a per-generation `--state-file`, a positive Windows `--direct-interface-index`, `--allow-external-probes`, and `--run-for` greater than zero and at most ten minutes. Model use still separately requires `--allow-model-api`. These flags authorize an operation; they do not assert path validity. The existing `observe` remains read-only.

First run `prepare-apply --config config.local.json --exclusive-controller --profile EFFECTIVE_CONFIG.yaml --output NEW_OWNERSHIP.json` against the explicitly owned controller with privately supplied `MIHOMO_SECRET`. This reads the controller and creates a ten-minute private lease; it makes no core update. The lease pins the generated file SHA256, ordered rules, controller/listener, proxy group, core PID/start time and a random pause token. A new process/config generation needs new ownership and a new journal. A crash lock must not be removed until the old observer is confirmed dead.

Windows read-only `Find-NetRoute` and `Get-NetAdapter -Physical` checks require the selected route to use the requested physical interface. The initial check inspects a numeric route without sending a packet; collection checks the actual resolved target before/after TLS, and commits recheck recorded target routes. Missing access, a preferred TUN route, unsupported OS or changed route blocks writes with an explicit reason. There is no attestation boolean or bypass. This checks OS route selection, not upstream quality; never change the host network to satisfy it.

Original rules and the first fallback connection remain untouched. Only fresh allowlisted terminal-MATCH TCP 443 observations can publish, once per host/run. Controlled TTL is capped at 60 seconds. Every provider PUT, including rollback, rechecks ownership, process identity and rules. Lease expiry permits only cleanup of the still-owned generation. No atomic core configuration epoch exists, so an uncoordinated app reload remains a stop/recovery event.

Before coordinated FlClash refresh, POST `/control/pause` on the agent listener with `Authorization: Bearer <pause_token>` from the private lease. Wait for a successful response containing `owned_providers_empty: true`: the worker has drained, the core's owned providers are empty, and the journal is paused. Only then refresh the app. This authenticated manual pause never rearms itself. Stop it, capture the new generation and restart with new ownership/journal. Missing/failed acknowledgment blocks the handoff. Abrupt termination can still leave core cache; use the explicit `run-controlled` supervisor below for independent empty recovery. No system service is installed.

New publication tests use local DNS/TLS/CONNECT and synthetic ownership/path-check doubles. Actual Windows physical-path checks, external/model calls and live controlled publication have not been executed. This implementation is ready for controlled acceptance, not production-accepted.

## Synthetic owned-core learning and recovery

For a separate, exclusively owned synthetic core only, use the source checkout's `scripts/smoke_observe.py` and its optional `--lifecycle`. See [OBSERVATION.md](OBSERVATION.md) and [RECOVERY.md](RECOVERY.md). The harness creates unused-loopback listeners and an isolated data directory, disables TUN, and uses `.test` fixtures plus a stub. Its local provider downloads explicitly bypass the synthetic SOCKS node using provider `proxy: DIRECT`.

`observe-lab` requires `--allow-lab-fixtures`, async/stub configuration, a fixed loopback controller, 1..128 bounded `.test` fixtures and `--state-file`. This synthetic mode publishes learned tail rules; controlled real collection uses the separate guarded path above. The first connection keeps original fallback; later TCP 443 connections can use a learned rule. Earlier original rules always take precedence. Restart clears previous learned entries before readiness; TTL and graceful exit clear owned providers while the core snapshot remains unchanged. Crashes can leave core cache until recovery; no supervisor is installed. Rule/provider APIs do not provide an atomic configuration epoch, so exclusive ownership remains required.

The older `serve`/DNS-Gate experiment remains separate and does not invoke the TLS collector. It must not be substituted for this Fake-IP observation path or pointed at a live configuration as part of package setup.

## Build, checks and remaining acceptance

```powershell
go test ./...
go vet ./...
go test -race ./...
./scripts/package.ps1
```

The package script requires Go and git in PATH, performs no installation, refuses existing outputs, and includes only an explicit list of program/docs/synthetic examples. The ZIP contains source revision/dirty status and SHA256 checksums. With dependencies already cached, set GOPROXY/GOSUMDB to `off` and GOTOOLCHAIN to `local` for offline builds. Race checks require CGO and a C compiler on Windows. CI runs local-endpoint Go tests/vet/build on Windows and Linux, plus Linux race; only dependency/action downloads require network. Core smoke is a separate manual check using an explicitly supplied core binary, never a CI download of user configuration.

Remaining live acceptance is environmental and integration work: prove independent direct/proxy routes in a separately owned environment, verify real evidence freshness/coverage, establish FlClash configuration ownership and refresh semantics, exercise crash supervision, and test actual TUN behavior. None was validated by these offline/local checks. No service, system DNS/proxy/TUN change, merge or release is part of this package.

### Offline supervisor boundary

`SuperviseOwned` serializes drain, confirmed exit, owner refresh and fresh worker acquisition. Unexpected exit permits one bounded owned-empty recovery, then returns an error without restarting learning. Unconfirmed exit blocks both recovery and refresh. Unit tests use in-memory worker/owner doubles only. The production FlClash configuration-owner adapter is not supplied; this library boundary does not enable automatic refresh. The separate `run-controlled` process supervisor below launches and supervises publication without refreshing FlClash. Independent concrete stopped-publisher recovery/watch commands are described below. The caller must retain exclusive ownership, bound refresh/start operations, obtain a new private lease/journal and clean up partial starts on failure.

### Concrete stopped-publisher recovery (offline tested, not live accepted)

`recover-apply` and `watch-recovery` use the real controller/provider adapter, not the `ConfigurationOwner` test interface. They never start FlClash, reload a profile, call a model, collect target evidence, terminate any process or restore learning. Use the same private config, lease, controller-secret environment and state path as the explicitly owned publisher. These are production entry points to review, not instructions to run against an unapproved current session:

```powershell
route-agent recover-apply --config private-agent.json --allow-owned-recovery --exclusive-controller --ownership private-ownership.json --state-file private-state.json
route-agent watch-recovery --config private-agent.json --allow-owned-recovery --exclusive-controller --ownership private-ownership.json --state-file private-state.json --run-for 10m
```

Existing config-loader flags still apply (a stub configuration also needs `--allow-lab-fixtures`); neither command grants external-probe/model permission. Both require controller authentication through `MIHOMO_SECRET`. Start the watcher separately after the publisher has created its versioned lock. It does not install a service or launch the publisher. It polls only that recorded PID, with a ten-minute CLI maximum; clean lock removal ends the watch without claiming recovery. A query error, cancellation or replaced lock stops it. A detected exit invokes empty recovery once, then exits without restarting learning. Failure of the watcher itself still requires explicit recovery.

New controlled publishers persist their PID in the lease lock before binding the provider endpoint. Recovery requires that PID to be absent (PID reuse therefore blocks), the exact lock to remain unchanged, and a separate exclusive recovery lock. Empty/legacy locks from older builds cannot prove process ownership and are rejected, never deleted automatically. Only an unchanged effective-config digest, core PID/start time, ordered rules, compatible private journal and free owned HTTP endpoint permit writes. Expired leases may be cleaned but never used to restart publication.

Recovery serves empty YAML from the same endpoint, PUTs only the two `route-agent-tail-*` providers, requires a fresh fetch of each body plus core metadata/count readback, and checks ownership again around updates. It writes `recovered_stopped` with zero journal entries. The old publisher lock remains as a consumed-lease tombstone: obtain new ownership before another publisher start. A partial failure retains that lock and reports failure; success is never inferred from rule count alone. An interrupted recovery leaves a recovery lock that requires explicit investigation. Snapshot checks are not an atomic core reload epoch.

Local tests use real loopback HTTP provider fetches, synthetic process/core identity checks, and no FlClash. Windows CIM process lookup and live core identity are still unaccepted. No real TUN or external request is part of these tests.

### Why automatic FlClash refresh is still blocked

Pinned official source `68c71b8ef9b7486a224972eb371ff153c6b2de0f` has several app-owned refresh origins: startup (`lib/bootstrap.dart:152`), a twenty-minute timer (`lib/application.dart:137`), GUI profile updates (`lib/providers/actions/profiles.dart:101`), scripts and provider/settings updates. Active-profile update calls `applyProfileDebounce` directly. Deep links in `lib/common/link.dart:35` accept `install-config`; the inspected dispatch has no external refresh/drain acknowledgement operation. `_setupConfig` writes generated YAML before calling core setup; its internal `preloadInvoke` callback is passed into core setup after that write, so it cannot be treated as an external pre-write hook.

The minimum reliable integration needs an app-owned barrier before generated-config replacement/core application for every relevant refresh origin: await authenticated agent drain, confirm publisher exit, apply the app's profile, then return an unambiguous completed-generation acknowledgement before preparing a fresh lease. A generated-file watcher observes replacement too late to provide this guarantee. Core `/configs` reload bypasses app ownership; `install-config` can add/replace a profile; neither is a safe substitute. The GUI selector passed the pinned synthetic app run 37740165846, but cannot intercept automatic or other manual changes.

No such application hook was changed or exposed. The no-fork constraint remains in force. The pinned QuickJS evaluator installs only console bindings and runs a pure JSON transform; pending I/O promises fail, so an overwrite script is not an authenticated external drain barrier. See [pinned evaluator](https://github.com/chen08209/FlClash/blob/68c71b8ef9b7486a224972eb371ff153c6b2de0f/plugins/rust_api/rust/src/script/mod.rs#L18).

The user selected configurable maintenance windows as a cooperative operating constraint. They reduce planned overlap but cannot supply atomic ownership: FlClash's startup/timer/manual/settings/script/core-restart paths do not wait for the agent. The agent does not edit FlClash's schedule or settings. `SuperviseOwned` remains an interface tested with doubles, without an automatic refresh CLI/owner implementation. Keeping unrestricted FlClash control would require an official upstream coordination hook or a different proven generation design; maintaining a private fork is not authorized. A separate possible no-fork design makes the agent sole owner of an unmodified Mihomo process, but changes the product workflow and still needs implementation.

## User-configured maintenance window (offline tested)

Before a controlled run, add `maintenance` to your separate private agent JSON. There is **no default time, duration, timezone or resume policy**, and no schedule is installed. Omitting the object disables this feature. Specify all four fields:

| Field | Accepted value |
|---|---|
| `start` | Daily local `HH:MM`, chosen by the operator before running |
| `timezone` | `Asia/Shanghai` (fixed UTC+08:00, Beijing time) or `UTC`; never inferred from Windows |
| `duration_seconds` | Integer 1..86399; cross-midnight windows are supported |
| `resume` | `manual` keeps the worker paused after the window; `unchanged` permits revalidation and resume of the same generation only |

Validate with `route-agent check --config PRIVATE.json` (stub configs additionally need `--allow-lab-fixtures`). Use the existing explicitly authorized `observe-apply` command/ownership guards. Other observer/probe/server modes reject this setting rather than silently ignoring it. Ordinary `observe-apply` retains its **ten-minute run and lease limits**. For windows beyond that bounded run, explicitly use `run-controlled` below: it renews short leases while identity remains unchanged, including during pause. Neither command installs a system startup task or changes FlClash scheduling.

The single write worker bounds observation work at the next window start and checks before nonempty provider PUTs, including rollback. An already accepted/in-flight core request cannot be retracted at the clock boundary. The worker then clears both owned providers, requires fresh downloads and metadata/count readback, and records the paused journal. Only completion exposes `/status` maintenance `paused_empty` (the last verified drain, not continuous attestation against external app changes); `scheduled` or `draining` is not permission to refresh. Startup inside the window reconciles empty before any learning. Empty recovery writes remain allowed. There is no precise wall-clock completion guarantee.

With `resume: manual`, stop the paused worker and explicitly prepare a fresh private lease/journal for the next generation. With `resume: unchanged`, window end rechecks lease expiry, effective-file digest, ordered runtime rules, core PID/start and recorded direct routes, reconciles empty again and starts observing only newer connections. It preserves the per-run attempt budget. Any changed/expired ownership blocks and stops; it never silently adopts a refreshed configuration or restarted core. Authenticated `/control/pause` overrides scheduled resume and remains a manual pause.

For cooperative alignment, the operator must arrange FlClash refresh after confirmed `paused_empty` and finish within the chosen window; this implementation does not configure the app or system tasks. A preset delay alone is not proof of drain. The pinned app's subscription refresh uses startup checks plus a twenty-minute timer and per-profile elapsed intervals, not an exact external acknowledgement schedule. Manual/startup/out-of-window refresh and core restart remain outside this protection. A refresh that changes identity requires new ownership even when it happens inside the window. A successful config-only app run does not validate this maintenance/publisher path.

Latest actual app evidence is [run 37740165846](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37740165846) at `89fd846f01b2956ed80afe8eea060f964e5ad9f7`: configuration/GUI refresh/repeat/persistence/exit passed with empty providers and TUN off. It predates maintenance code and did not exercise controlled learning or recovery/watch commands. See ACCEPTANCE.md for the remaining bounded acceptance proposal; no new application/TUN run is authorized.

## Continuous supervised operation

`run-controlled` is a foreground product entry point that starts an independent publishing child and supervises its lifetime. It has no ten-minute process timeout, but requires **both** `--allow-continuous` and `--allow-owned-recovery`, plus every normal publication/ownership/physical-route permission. The ordinary bounded entry remains unchanged. Do not supply `--run-for` to the continuous command.

The initial private lease is still captured with `prepare-apply`. Before active observation the child reduces it to a two-minute renewable lease; it renews when at most one minute remains. Renewal verifies the current private lease/file digest, ordered runtime rules, core PID/start and recorded direct routes. It atomically replaces only the expiry of the same ownership generation, after rechecking identity and context. It never revives an expired lease, adopts changed configuration or lengthens a lease indefinitely. Every nonempty provider write retains its own checks. Renewal/disk/identity failure stops publication and attempts verified empty cleanup; failed cleanup retains the PID lock for independent recovery.

The parent sends a pipe heartbeat every second. EOF, an invalid frame or five seconds without heartbeat cancels the child's work and drains. On Ctrl+C, the parent closes that pipe and waits for normal exit for up to twenty seconds; only an unresponsive child it launched can then be killed. Only after `Wait` confirms child exit does the parent perform one bounded empty recovery, reading the latest renewed lease and verifying the recorded PID and unchanged core. A crash never automatically restarts learning. If the parent dies, the child detects heartbeat loss; if both die, use the existing explicit `recover-apply` after establishing process absence. No service, system task or FlClash/core process launcher is installed.

The observer continues to check identity while paused. Changing the profile/core, even during the window, stops the old generation rather than adopting it. For an actual refresh, wait for verified drain, stop the supervised session, perform the owned refresh, then capture **new ownership and journal paths** and start a new session. An unchanged window may resume automatically if `resume: unchanged`. The supervisor does not configure or initiate FlClash refresh. Unexpected manual/startup/out-of-window changes are rejected when observed, with the already-documented non-atomic check/PUT interval; the operator must preserve cooperative exclusive control. This implementation does not add the unconditional refresh owner the user replaced with maintenance windows.

Continuous collection can reconsider an allowlisted host only after a one-minute cooldown and a **new** eligible fallback connection after that cooldown. TTL remains at most one minute. The configured `max_api_requests` remains a finite **whole-session attempt budget**, never silently reset by renewal, a pause or an internal restart (there is no automatic restart). Each attempt invokes the model at most once. Exhaustion prevents new attempts while TTL cleanup and supervision continue; `/status` exposes `continuous`, `attempt_budget`, `budget_exhausted`, counters and maintenance state. An operator-started new session has a new budget; there is no cross-session dollar cap. Set a deliberate budget and approved provider/account controls before real model use.

Prerequisites: an explicitly owned Windows core/controller with authentication and the two expected HTTP tail providers; private writable lease/journal directory; unchanged effective configuration file; approved exact target allowlist/DNS/proxy; a verified direct physical interface and independently established proxy path; operator-selected maintenance configuration; and separately approved model credentials when using Jev. A stub configuration remains UNCERTAIN and cannot prove real learning. The daemon does not apply a profile, select a NIC, change DNS/TUN/proxy, read user subscriptions or supply these prerequisites for you.

After configuring the private JSON and supplying `MIHOMO_SECRET` privately in the process environment, these are the actual startup commands (`$EffectiveConfigPath` and `$DirectInterfaceIndex` are values verified for the explicitly owned environment):

```powershell
./route-agent.exe check --config ./private/agent.json
./route-agent.exe prepare-apply --config ./private/agent.json --exclusive-controller --profile $EffectiveConfigPath --output ./private/session-ownership.json
./route-agent.exe run-controlled --config ./private/agent.json --allow-continuous --allow-owned-recovery --allow-controlled-apply --exclusive-controller --ownership ./private/session-ownership.json --state-file ./private/session-state.json --direct-interface-index $DirectInterfaceIndex --allow-external-probes --allow-model-api
```

The last flag and privately supplied `OPENROUTER_API_KEY` are required only for the separately authorized Jev configuration. For stub observation, omit that flag/key and add `--allow-lab-fixtures` to each config-loading command; no nonempty real decision will result. Stop with Ctrl+C and confirm normal empty cleanup. A crashed/interrupted publisher or supervisor leaves lock evidence: do not delete it to force reuse; investigate/recover and use fresh ownership after verified shutdown.

Offline tests advance an injected lease clock over thirty-one minutes, including a long maintenance pause/resume, without sleeping for simulated time. They cover expired/changed/missing ownership refusal, actual local test-child crash/normal exit and heartbeat EOF/invalid/missing frames. Loopback provider recovery tests cover fresh fetch/readback. These checks establish implementation behavior, not live Windows acceptance or physical/model evidence.

## Prepared single isolated lifecycle acceptance (not executed)

`python -B scripts/lifecycle_acceptance.py` prints an inert plan. The exact-label workflow `.github/workflows/owned-windows-lifecycle.yml` requires a separately approved addition of `accept-lifecycle-on-hosted-windows` to PR #5; normal pushes and CI do not start it. No label was added in this implementation.

The one proposed campaign uses a standard disposable `windows-2025` guest, exact approved checkout SHA and a twenty-minute job limit. It downloads the pinned official Mihomo v1.19.32 ZIP (SHA256 `1ac84e795b5b915446e20139677fb6cc013a0778e876e8d4c96f994750b4c4a3`), builds the product plus two **separate Go test binaries**, and installs existing pinned smoke dependencies only in a guest venv. It starts one standalone owned core with TUN disabled, loopback fixture DNS/SOCKS/echo/provider endpoints and synthetic `.route-lab.test` traffic. No FlClash application, host settings, service, task, firewall, default route, real target or model API is involved.

The test-only worker injects synthetic reachability, judge and physical-path checks; these bypasses are absent from the product executable. It retains actual Windows core PID/start verification, provider APIs, renewed leases, maintenance worker, heartbeat protocol and empty recovery. The test-only parent calls the same production process supervisor/recovery functions. Thus success would validate **synthetic learning and lifecycle integration**, not the full real-evidence product path.

One campaign checks original/first fallback, later learned TCP 443 and other-port preservation; a guest-only near-term maintenance window with verified empty providers and unchanged resume; one deliberate owned supervisor kill, requiring heartbeat-loss child cleanup; then a fresh owned session and one deliberate publisher kill, requiring independent empty recovery and no restart. The script records binary SHA256, tested commit, results, recorded-PID absence, fixture listener removal and unchanged guest network snapshots. It uploads no artifacts/caches and prints only synthetic result JSON. Missing prerequisites or failed checks stop that attempt; no automatic retry is authorized. Real model calls, independent physical paths, actual FlClash integration and TUN remain separate unaccepted scopes.
