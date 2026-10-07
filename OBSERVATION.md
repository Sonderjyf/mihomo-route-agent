# Isolated asynchronous connection learning

The chosen behavior retains Fake-IP and original rules: the first connection uses the original fallback; background learning can affect later connections. `observe-lab` is the first functional experiment for this behavior, not a supported live FlClash integration. It accepts only `--allow-lab-fixtures`, async mode, stub judging, an explicit loopback controller and 1..128 allowlisted `.test` fixtures. It neither reads API credentials nor uses the DNS Gate, real TLS probes or the model API.

The harness creates an inert tail-preview from a synthetic profile, then explicitly changes only its owned tail providers to HTTP URLs served by the observer. The regular `tail-preview` remains a private offline plan with empty file providers. Only the harness-created core receives provider PUTs. Start `observe-lab` only with an owned isolated core; do not point it at installed FlClash.

## Evidence and decision path

The observer reads `/configs`, `/rules` and `/connections`. It requires rule mode, one final unconditional `Match`, the two expected tail rules immediately before it, enabled rules and a supported straight-line rule sequence. It pins the stable rule metadata, excluding changing hit counters, and rechecks before each poll and provider commit. Nested/sub-rule/rematch/unknown types fail closed. It does not reproduce Mihomo's matching logic.

A connection is eligible only when it started after observer readiness, reports `rule=Match`, an empty `rulePayload`, and a chain ending at the pinned fallback target. It must be TCP to destination port 443 with a valid nonlocal hostname in the synthetic allowlist. IP literals, absent hosts, explicit domain/provider hits, special proxy/rule contexts and sniffed host contexts are skipped. Core rule metadata thus supplies the routing evidence; host spelling supplies no reachability evidence. Both managed core rules also require TCP/443 via AND conditions, so evidence cannot spill into UDP or other ports.

One bounded worker builds the existing `State` from labeled synthetic evidence, calls the stub and passes its answer through `Accept`. The current lab stub proposes PROXY; it is accepted only for repeated direct failure plus verified proxy success. Untested evidence remains UNCERTAIN. Each allowlisted host gets at most one attempt per run; configured request budget and provider capacity still apply. There is no retry scheduler. Late/canceled results do not publish. Successful decisions use the existing full-snapshot provider PUT/ACK/count-readback and rollback mechanism, under the separate `route-agent-tail-` namespace. Expired entries are pruned while the observer is running.

## Reproduce without user traffic

Use Go to build the Agent and an already available separate official core binary. Python dependencies are the existing smoke requirements. The work directory must not exist. The harness checks all laboratory ports before creating files or launching children, strips inherited API/controller secrets, uses only local DNS/SOCKS/echo endpoints, and stops only processes it created. Optionally supply a local JSON list of protected ports with `--protected-ports`; that file is not copied into evidence.

~~~powershell
go build -trimpath -o dist/route-agent.exe ./cmd/route-agent
python scripts/smoke_observe.py --agent dist/route-agent.exe --mihomo C:/tools/mihomo.exe --workdir local-evidence/observer-new-run
~~~

Verified with official Mihomo v1.19.32 on Windows:

- A local DNS query returns Fake-IP; the SOCKS client then connects to that IP, and the core recovers the synthetic hostname.
- The first established connection reports `Match`, empty payload and `BASE` chain.
- A later TCP 443 connection reports `AND`, the TCP/443/`route-agent-tail-proxy` payload and `LEARNED`; the first open connection still reports its original `BASE` chain. The same host on TCP 8443 remains `Match`/`BASE`.
- A configured domain rule retains precedence and triggers no judgment. A no-evidence fallback host gets no learned rule. Two judgments produce one commit/entry.
- Changing only the owned core's mode causes the observer to exit. Mock tests also cover rule drift during judgment, timeout, duplicates, stale connections and ambiguous metadata.

See `evidence/observer-scoped-core.json` and `evidence/observer-scoped-lifecycle.json` for current synthetic results and core hash. Older `observer-core-2026-10-07.json` records the prior unscoped milestone. This is actual core routing evidence using mock reachability evidence, not a real network-quality decision or TUN test. The separate real-collector `observe` command is always read-only shadow; its local tests and usage are described in [RUNBOOK.md](RUNBOOK.md).

## Limits before live acceptance

The checked core implementation creates a TCP tracker after an outbound connection is established. `/connections` is a snapshot of current trackers, including its WebSocket form. Failed dials and connections that close between samples may be absent; this mechanism cannot promise to learn from every failed first connection. UDP/QUIC and sniffed/special-rule flows are deliberately not supported by this milestone.

Connection records do not carry the matched rule index or a configuration generation. Unique terminal MATCH plus stable snapshots is sufficient for the controlled experiment, but there is no atomic fence between checking configuration and committing providers. Concurrent reloads/restarts can invalidate that inference. General live support requires a reliable configuration identity/exclusive integration contract and tests for FlClash's settings pass and subscription refresh. Provider count readback does not prove content identity by itself; the smoke separately audits the later connection's matched provider.

The observer is not a production service lifecycle solution. The bounded [restart/expiry experiment](RECOVERY.md) now journals state, evicts all learned entries before restart readiness, and cleans up on graceful exit only while its core-rule fingerprint still matches. A killed process cannot clean itself, and the core may continue using cached entries until reconciliation. There is no external supervisor or offline TTL enforcement. Before live acceptance, add real bounded evidence collection and model-policy tests, stronger provider identity/configuration fencing, missed/failed connection coverage, and a separate controlled trial with explicit profile/rollback ownership. No current result authorizes modifying active FlClash, system DNS/proxy or TUN.
