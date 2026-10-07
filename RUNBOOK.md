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

## Owned-core learning and recovery

For a separate, exclusively owned synthetic core only, use the source checkout's `scripts/smoke_observe.py` and its optional `--lifecycle`. See [OBSERVATION.md](OBSERVATION.md) and [RECOVERY.md](RECOVERY.md). The harness creates unused-loopback listeners and an isolated data directory, disables TUN, and uses `.test` fixtures plus a stub. Its local provider downloads explicitly bypass the synthetic SOCKS node using provider `proxy: DIRECT`.

`observe-lab` requires `--allow-lab-fixtures`, async/stub configuration, a fixed loopback controller, 1..128 bounded `.test` fixtures and `--state-file`. Only this mode publishes learned tail rules. The first connection keeps original fallback; later TCP 443 connections can use a learned rule. Earlier original rules always take precedence. Restart clears previous learned entries before readiness; TTL and graceful exit clear owned providers while the core snapshot remains unchanged. Crashes can leave core cache until recovery; no supervisor is installed. Rule/provider APIs do not provide an atomic configuration epoch, so exclusive ownership remains required.

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
