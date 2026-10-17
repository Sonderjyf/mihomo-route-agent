# Preserve core routing semantics

`route-agent assess --profile separate-copy.yaml --output new-report.json` reads an explicit YAML/JSON copy without loading Agent configuration, binding ports, calling controllers or looking up credentials. It emits counts, fixed categories and fixed findings, with no node names, addresses, rule operands or source contents. Unknown rule/proxy labels become `OTHER`; parse errors omit source text. The report does not validate core syntax or prove preview compatibility. Existing files are never overwritten. Keep source copies private; do not publish personal profiles as test fixtures.

The strict `preview` remains a synthetic experiment. Real profiles can combine process rules, IP/GeoIP, nested logic, dynamic providers, DNS policies, Fake-IP and protocols beyond SOCKS5. Dropping these fields to make preview pass would change routing. Assessment does not authorize conversion or application.

## Recommended architecture

Mihomo owns ordered evaluation of the complete original rules and connection context. A future adapter should preserve the original rule list and opaque proxy/provider fields. It should only add learning at an explicitly defined fallback boundary, immediately before the final catch-all, after all original specific rules. Profiles with other control flow or no unambiguous fallback must be rejected for review. Existing decisions retain precedence; the Agent must not classify unsupported rule semantics as UNKNOWN or expand them into its DNS-only matcher.

`tail-preview` implements the **offline fallthrough adapter**, separate from the existing limited preview: it preserves every original rule string and its order, retains opaque node/provider and DNS fields, and inserts two collision-checked file-provider references immediately before the final unconditional MATCH. It rejects earlier MATCH, sub-rules, logical/unknown control flow, ambiguous proxy targets and repeated injection. This bounded command does not validate every original rule's core syntax.

~~~powershell
route-agent tail-preview --profile examples/isolated-profile.yaml --proxy-target PROXY --output new-tail-plan.json
~~~

The artifact contains `candidate`, `provider_files` (two empty local YAML payloads), zero-based `insert_at`, and limitations. Only the new plan JSON is written; provider files are included as strings, not created. Existing outputs cannot be overwritten. It does not need Agent configuration. File providers intentionally have no connection to the current Agent HTTP service, so this command chooses neither async nor preflight behavior and cannot enable learning. Like preview, the artifact contains original credentials and must remain private.

Managed references use `AND,((NETWORK,tcp),(DST-PORT,443),(RULE-SET,...)),TARGET`. The same hostname on UDP or another destination port continues through the original policy. The real collector is connected only to shadow commands; only the synthetic owned-core observer can publish rules. Current commands and packaging are in [RUNBOOK.md](RUNBOOK.md).

Synthetic tests check process/IP/GeoIP/domain rule order, opaque protocol/DNS fields, provider preservation and collision rejection. The accepted async behavior is demonstrated by the isolated controller/connection observation harness in OBSERVATION.md. Never start a candidate that retains live listeners, TUN, endpoints or provider URLs. A separate lab execution copy must replace those with explicit synthetic endpoints and nonconflicting loopback ports. The adapter alone cannot prove correct runtime learning.

Validation on Windows, 2026-10-07: offline Go test/vet/race passed. Official Mihomo v1.19.32 `-t` accepted a synthetic candidate with local empty file providers and process/IP/domain rules, using a separate data directory. GeoIP order is covered by a structural unit test; this increment did not load a GeoIP database or test its routing. No core routing service, controller observation, real node, API or TUN was exercised by the tail increment.

Connection observation must first establish that the original core policy reached the fallback, and that a reliable domain is available. A rule/provider match, original explicit decision, ambiguous result or missing domain must not trigger learning. DNS observations alone cannot establish this for process/IP rules. Validate the chosen core version's connection metadata before implementing this decision; do not assume connection events expose enough information.

## Product decision: Fake-IP and the first connection

Keeping Fake-IP and existing DNS policies uses **asynchronous learning from core connection observations**. The first connection follows the existing fallback; a verified learned rule can affect later connections. This behavior has been accepted for the project and is now demonstrated in the [isolated observer milestone](OBSERVATION.md). It does not promise a model decision before the first connection. The milestone uses only synthetic evidence/stub judging and does not authorize live configuration changes.

If a decision must precede the first connection, changing DNS to redir-host in a separate experimental profile is one possible experiment, but DNS-first gating still cannot evaluate process context or guarantee coverage for direct-IP connections, application DoH, cached answers or host overrides. Strict connection-level preflight needs a supported core hook/forwarding architecture and separate latency/failure design. This tradeoff requires a product decision; a silent Fake-IP-to-redir-host conversion is not an adapter.

FlClash can apply application settings after profile scripts. Generated disk configuration and saved preferences therefore do not establish runtime state or prove that a script survives the settings pass. Controlled FlClash acceptance, refresh behavior and actual TUN testing remain separate stages requiring authorization to change the live setup. Offline work does not require those changes.
