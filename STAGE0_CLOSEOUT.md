# Stage 0 closeout — evidence alignment (2026-10-09)

This closes documentation alignment for PR #5, not production acceptance. The reviewed starting head was `9a7f941967fb65aacff51b753758847692ff62be`; the stacked base is `codex/go-route-agent-prototype` at `c75166913af788107eece66406f9e16ee6ceecb2`. Product source remains `b4f6f18fbbac6edb83093484934fe8d2e8448f07`. The documentation commit's own SHA is recorded by Git; it is not substituted for any execution SHA.

## Completed work

- [DELIVERY_STATUS](DELIVERY_STATUS.md) is the current acceptance source, with separate DIRECT, PROXY, provider publication, learned connection, FlClash, TUN, maintenance, recovery and continuous-operation statuses.
- [ACCEPTANCE](ACCEPTANCE.md) retains failed/synthetic campaigns and links to that canonical matrix instead of treating synthetic publication as real learned routing.
- [RUNBOOK](RUNBOOK.md) distinguishes nonpublishing connectivity validation from production publication, current native identity from the historical twelve-second PowerShell repair, and file preview from HTTP runtime providers.
- [BUILDING](BUILDING.md) provides a current offline entry point and explicitly dates/preserves the old DNS-Gate instructions.
- PR #5's description is aligned with these capabilities and exact evidence boundaries; the PR remains Draft and stacked on the prototype branch. Merge, release and production enabling are separate decisions.

## Refusal investigation closed within its evidence limits

[Run 37887426801](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37887426801), execution `b4f6f18fbbac6edb83093484934fe8d2e8448f07`, establishes `probability_below_threshold` for both DIRECT choices, accepted as UNCERTAIN. This is a normal refusal under the unchanged 0.8 selected-probability gate. Explicit VLESS TLS success does not replace production proxy evidence, which remained `not_tested`.

[Run 37880640439](https://github.com/Sonderjyf/mihomo-route-agent/actions/runs/37880640439), execution `158fc35122d0bf0be74285737071db22d9f8ed8d`, did not log the refusal branch; its retrospective status remains `reason_unknown`. The later result cannot fill the missing historical probability/contract information. The earlier failed run `37867539248` still has unknown attempts/spend/cleanup; its JSON record is unchanged.

No repeat model request is needed to close this investigation. No threshold, confidence substitution, target scope, model or route behavior changed. Masked JSON was not reconstructed; full real-run JSON remains independently unvalidated.

## Validation and retained boundary

Stage 0 checks only documentation diff scope, Markdown links, run/SHA correspondence, retained historical evidence and product-tree equality. It does not add tests or start an acceptance campaign. A documentation push may run the existing ordinary offline PR CI; that is not a real model/core/app/TUN acceptance run. No workflow file, launcher pin, secret or package was changed by this stage.

The completed level is **L0: default-disabled experimental delivery; connectivity and refusal verified; real learning unaccepted**. An accepted production decision, nonempty provider ACK/readback and associated later real connection are still needed. The next implementation scope is offline production diagnostics/deadline visibility and an inert single-target acceptance harness; this document does not authorize that implementation or any real execution.
