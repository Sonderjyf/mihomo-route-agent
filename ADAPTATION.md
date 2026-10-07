# Preserve core routing semantics

`route-agent assess --profile separate-copy.yaml --output new-report.json` reads an explicit YAML/JSON copy without loading Agent configuration, binding ports, calling controllers or looking up credentials. It emits counts, fixed categories and fixed findings, with no node names, addresses, rule operands or source contents. Unknown rule/proxy labels become `OTHER`; parse errors omit source text. The report does not validate core syntax or prove preview compatibility. Existing files are never overwritten. Keep source copies private; do not publish personal profiles as test fixtures.

The strict `preview` remains a synthetic experiment. Real profiles can combine process rules, IP/GeoIP, nested logic, dynamic providers, DNS policies, Fake-IP and protocols beyond SOCKS5. Dropping these fields to make preview pass would change routing. Assessment does not authorize conversion or application.

## Recommended architecture

Mihomo owns ordered evaluation of the complete original rules and connection context. A future adapter should preserve the original rule list and opaque proxy/provider fields. It should only add learning at an explicitly defined fallback boundary, immediately before the final catch-all, after all original specific rules. Profiles with other control flow or no unambiguous fallback must be rejected for review. Existing decisions retain precedence; the Agent must not classify unsupported rule semantics as UNKNOWN or expand them into its DNS-only matcher.

The next implementation is an **offline fallthrough adapter**, separate from the existing limited preview: preserve every original rule byte-for-byte and in order; retain opaque node/provider and DNS objects; add collision-checked managed provider references at the agreed fallback boundary. Verify with synthetic process/IP/GeoIP/domain rules, a local provider fixture, core syntax checks, and an isolated controller/connection observation harness. Never start a candidate that retains live listeners, TUN, endpoints or provider URLs. A separate lab execution copy must replace those with explicit synthetic endpoints and nonconflicting loopback ports. The adapter alone cannot prove correct runtime learning.

Connection observation must first establish that the original core policy reached the fallback, and that a reliable domain is available. A rule/provider match, original explicit decision, ambiguous result or missing domain must not trigger learning. DNS observations alone cannot establish this for process/IP rules. Validate the chosen core version's connection metadata before implementing this decision; do not assume connection events expose enough information.

## Product decision: Fake-IP and the first connection

Keeping Fake-IP and existing DNS policies favors **asynchronous learning from core connection observations**. The first connection follows the existing fallback; a verified learned rule can affect later connections. This is the recommended next isolated prototype because it preserves current DNS behavior. It does not promise a model decision before the first connection, and API/evidence collection remains disabled in offline tests.

If a decision must precede the first connection, changing DNS to redir-host in a separate experimental profile is one possible experiment, but DNS-first gating still cannot evaluate process context or guarantee coverage for direct-IP connections, application DoH, cached answers or host overrides. Strict connection-level preflight needs a supported core hook/forwarding architecture and separate latency/failure design. This tradeoff requires a product decision; a silent Fake-IP-to-redir-host conversion is not an adapter.

FlClash can apply application settings after profile scripts. Generated disk configuration and saved preferences therefore do not establish runtime state or prove that a script survives the settings pass. Controlled FlClash acceptance, refresh behavior and actual TUN testing remain separate stages requiring authorization to change the live setup. Offline work does not require those changes.
