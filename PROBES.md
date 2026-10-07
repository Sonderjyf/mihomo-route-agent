# Bounded TLS evidence library

`NewTLSCollector(dnsAddress, proxyURL, attemptTimeout)` implements real socket/TLS evidence collection. `EvaluateEvidence(ctx, hostname, collector, judge)` connects it to the existing `Judge` interface, including `NewJev`, and applies `Accept` to the collected State. This milestone is a callable library with local endpoint tests. It is **not activated by any CLI, Agent or observer command**; no background probes, real model calls or provider updates are introduced automatically.

## Collected evidence

The DNS endpoint must be an explicit numeric IP:port. The collector sends A and AAAA queries directly there, retrying truncated UDP over TCP, without consulting system hosts/search domains. Missing/failed DNS or more than 16 returned addresses leaves evidence unavailable. Any private/local/protected or 198.18.0.0/15 Fake-IP answer rejects the entire result, including mixed public/private responses. These checks do not establish DNS authenticity.

The direct path pins the first returned address to TCP/443 and uses the original hostname for SNI and certificate hostname verification. It performs at most two attempts; one valid TLS handshake establishes direct success and stops collection. Only two transport connect failures/timeouts produce `repeated_failure`. Certificate validation failure or another TLS/protocol error remains uncertain and is not converted into repeated reachability failure.

After two direct transport failures, the collector makes one HTTP CONNECT attempt through the explicitly configured `http://127.0.0.1:PORT` proxy. CONNECT carries the same pinned numeric address and port, avoiding a different/protected result from proxy-side DNS. TLS still uses and verifies the original hostname. The tunnel must return 200, then complete a valid TLS handshake. Proxy refusal, proxy connection/protocol failure, certificate error and TLS failure remain separately visible in a fixed-vocabulary `ProbeReport`. They do not become evidence that direct access works.

Both paths use normal certificate validation and TLS 1.2 minimum; the default trust store is the operating system's. There is no insecure-validation flag or exported test-root override. Tests inject an ephemeral local server certificate and local dial mapping inside the package. No application HTTP request, URL path, cookies or page content is sent to destination servers. CONNECT response headers are bounded to 16 KiB, with a 4 KiB line limit.

Each attempt has a configured 50 ms..5 s bound, collection has an overall four-attempt-timeout bound, and `EvaluateEvidence` caps the combined collection/model operation at 10 seconds or the caller's shorter deadline. Cancellation closes blocked connections. Model invocation occurs at most once and only when evidence can support a decision; there are no automatic model retries. The existing acceptance policy still vetoes an answer unsupported by the supplied facts.

## Model boundary and local validation

Only normalized hostname, registrable domain and the two coarse evidence labels enter Jev's existing `State` request. Detailed local errors, DNS answers, IP addresses, certificates and timings are not sent. The existing recipient is `https://openrouter.ai/api/alpha/decisions`, requesting `typesafe/jev-1.13`; this increment adds no credentials or endpoint override. An in-memory HTTP transport tests the actual Jev request serializer and response parser without making an API call.

Offline Windows Go test/vet/race and build validate:

- Explicit local DNS plus real local TLS with certificate verification on both direct and CONNECT paths.
- Direct success stops after one attempt; two synthetic direct connect failures plus valid proxy TLS can pass a PROXY answer through policy.
- Unknown CA, wrong certificate hostname, proxy 503, mixed private/Fake-IP answers and canceled TLS do not trigger model calls or a routing decision.
- The destination receives no application HTTP request. Jev receives only the expected compact State through the in-memory stub.

There is no claim of an actual external reachability result. A TLS success is scoped to one hostname/port/address/path/time, not application usefulness or every address. Two failures are not proof of censorship. Address selection and transient or route-specific network problems can affect the result. A direct socket also does not prove bypass of an existing system TUN; operating-system routing can intercept it. A proxy's actual routing policy must also be independently established.

## Concrete external test proposal — not executed or authorized here

Before any external run, obtain explicit approval for all of the following:

1. Destinations: only `example.com:443` and `www.cloudflare.com:443`, with TLS handshake only and no HTTP request. These are proposed test destinations, not claims about their current availability. Query A/AAAA only through one separately approved numeric DNS resolver endpoint. Reject private/Fake-IP answers as implemented.
2. Paths: a separately owned experimental environment with a verified direct route and one explicitly identified HTTP CONNECT proxy. Do not disable/change the user's active TUN, FlClash, system DNS or proxy to create that environment. If independent routes cannot be established, label results path-ambiguous and do not learn routes.
3. Data/recipient: at most the two listed names, registrable names and coarse TLS evidence labels to OpenRouter's Decisions API for Jev/its serving provider. No user traffic, browsing history, resolved addresses, raw error strings, certificates or credentials in prompts/logs. The API key, if later approved and configured, is used only in its authorization header and never stored in evidence.
4. Bounds: one collector evaluation per approved host; at most two direct TCP/TLS attempts and one proxy CONNECT/TLS attempt per host, A/AAAA with at most one truncation retry each, no model retries and at most two model requests in total. Use a caller deadline of 10 seconds and explicit low attempt timeouts. No provider publication or background monitoring during that external evidence trial.
5. Spending: proposed total ceiling US$0.01, subject to explicit approval and current provider pricing/account limits being verified first. A request-count cap is not a hard monetary cap; the current client does not enforce dollar spend. If the approved ceiling cannot be guaranteed with provider/account controls, perform the TLS-only phase and keep the model stubbed. No historical price estimate is treated as a guarantee.

Before activating this collector in the observer, decide how TCP/443 evidence may affect domain-wide rules (or limit the learned rule's port scope), establish route independence and evidence freshness, and retain existing original-rule precedence. These production policy/integration choices and real external trials remain pending; the library does not preempt them.
