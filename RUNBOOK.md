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

## Real evidence: shadow only

`examples/observation.shadow.json` is an inert configuration example, with lab-port placeholders. `check` validates syntax, not connectivity or route independence. Configure your own explicitly authorized DNS endpoint, HTTP CONNECT proxy, controller, unused status port and exact host allowlist in a separate private copy before running:

```powershell
./route-agent.exe probe --config config.local.shadow.json --allow-lab-fixtures --allow-external-probes example.com
./route-agent.exe observe --config config.local.shadow.json --allow-lab-fixtures --allow-external-probes --run-for 30s
```

These commands perform network activity only with `--allow-external-probes`. The example uses the stub judge, so `--allow-lab-fixtures` is also required; the stub always returns UNCERTAIN. `probe` accepts one exact allowlisted hostname and prints coarse collected evidence plus a candidate decision. `observe` reads the configured controller, collects evidence only for allowlisted fresh TCP 443 terminal-MATCH connections, and serves aggregate `/status` counters. It has no provider endpoint, provider writes or journal. The actual collector is wired to both commands; neither can apply a real-evidence decision.

Each observation run attempts each host at most once and no more than `max_api_requests` hosts. Each eligible attempt can make at most one model request; collection/model work has a 10-second maximum and respects shorter cancellation/deadlines. `--run-for` bounds observer lifetime to up to ten minutes when specified; zero is foreground operation until canceled. Failed or missed snapshots are not retried per host. Changing core mode/rule metadata stops observation. Counts reset each process; no cross-restart monetary budget is implemented.

To use Jev, choose `judge: "jev"`, configure any required explicit `api_proxy`, and separately supply `--allow-model-api`. Only after that flag is checked does the command read `OPENROUTER_API_KEY`; never put its value in a configuration, command argument, report or repository. `MIHOMO_SECRET` is optional controller authentication for shadow reads. Model state contains hostname, registrable name and coarse TLS evidence labels. The model endpoint is OpenRouter Decisions / `typesafe/jev-1.13`. A request count is not a hard dollar cap; account controls and separate approval are required for a paid trial. No model call is necessary for local tests or package checks.

TLS uses verified SNI/certificates, pins a numeric DNS answer across direct/CONNECT attempts, rejects protected/Fake-IP answers, and sends no application request to target websites. See [PROBES.md](PROBES.md). A normal socket can still traverse an active TUN: `--allow-external-probes` does not certify a physical direct path. The current machine's preflight could not establish independent paths, so external validation stopped before target DNS/TLS/model requests. Real-evidence routing remains deliberately unavailable.

## Controlled publication and refresh handoff

`observe-apply` connects the real collector to the bounded TCP 443 publisher. It requires `--allow-controlled-apply`, `--exclusive-controller`, a private `--ownership` file, a per-generation `--state-file`, a positive Windows `--direct-interface-index`, `--allow-external-probes`, and `--run-for` greater than zero and at most ten minutes. Model use still separately requires `--allow-model-api`. These flags authorize an operation; they do not assert path validity. The existing `observe` remains read-only.

First run `prepare-apply --config config.local.json --exclusive-controller --profile EFFECTIVE_CONFIG.yaml --output NEW_OWNERSHIP.json` against the explicitly owned controller with privately supplied `MIHOMO_SECRET`. This reads the controller and creates a ten-minute private lease; it makes no core update. The lease pins the generated file SHA256, ordered rules, controller/listener, proxy group, core PID/start time and a random pause token. A new process/config generation needs new ownership and a new journal. A crash lock must not be removed until the old observer is confirmed dead.

Windows read-only `Find-NetRoute` and `Get-NetAdapter -Physical` checks require the selected route to use the requested physical interface. The initial check inspects a numeric route without sending a packet; collection checks the actual resolved target before/after TLS, and commits recheck recorded target routes. Missing access, a preferred TUN route, unsupported OS or changed route blocks writes with an explicit reason. There is no attestation boolean or bypass. This checks OS route selection, not upstream quality; never change the host network to satisfy it.

Original rules and the first fallback connection remain untouched. Only fresh allowlisted terminal-MATCH TCP 443 observations can publish, once per host/run. Controlled TTL is capped at 60 seconds. Every provider PUT, including rollback, rechecks ownership, process identity and rules. Lease expiry permits only cleanup of the still-owned generation. No atomic core configuration epoch exists, so an uncoordinated app reload remains a stop/recovery event.

Before coordinated FlClash refresh, POST `/control/pause` on the agent listener with `Authorization: Bearer <pause_token>` from the private lease. Wait for a successful response containing `owned_providers_empty: true`: the worker has drained, the core's owned providers are empty, and the journal is paused. Only then refresh the app. The paused process serves empty providers and never rearms itself. Stop it, capture the new generation and restart with new ownership/journal. Missing/failed acknowledgment blocks the handoff. Abrupt termination can still leave core cache; no production supervisor is installed.

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

`SuperviseOwned` serializes drain, confirmed exit, owner refresh and fresh worker acquisition. Unexpected exit permits one bounded owned-empty recovery, then returns an error without restarting learning. Unconfirmed exit blocks both recovery and refresh. Unit tests use in-memory worker/owner doubles only. The production FlClash configuration-owner adapter is not supplied; this library boundary does not enable automatic CLI refresh or startup. Independent concrete stopped-publisher recovery/watch commands are described below. The caller must retain exclusive ownership, bound refresh/start operations, obtain a new private lease/journal and clean up partial starts on failure.

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

The minimum reliable integration needs an app-owned barrier before generated-config replacement/core application for every relevant refresh origin: await authenticated agent drain, confirm publisher exit, apply the app's profile, then return an unambiguous completed-generation acknowledgement before preparing a fresh lease. A generated-file watcher observes replacement too late to provide this guarantee. Core `/configs` reload bypasses app ownership; `install-config` can add/replace a profile; neither is a safe substitute. The current GUI selector has not yet passed real release validation and cannot intercept automatic or other manual changes.

No such application hook was changed or exposed in this task. Connecting it requires either an authorized FlClash source integration/build, or an explicitly exclusive app-control arrangement plus successful real GUI validation covering all refresh origins. Those are unresolved prerequisites, not an invitation to change the user's current app settings. `SuperviseOwned` therefore remains unconnected to application refresh; only the independent real recovery/watch entries above are connected.

The completed second application attempt tested exactly commit `66cadea7ca91786bed3a9ca60ee234d6892ae32c` and failed at GUI navigation. Its one-run approval is consumed; no further actual execution is authorized. These new Go recovery/watch commands are outside that app-only acceptance. Testing them against real process/core ownership would require separate explicit scope, so they have only offline synthetic evidence here.
