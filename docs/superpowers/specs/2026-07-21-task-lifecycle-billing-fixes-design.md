# Task Lifecycle and Billing Fixes

## Goal

Repair the async video/task merge without changing the task adaptor interface or public API shape.

## Design

- Keep `Task.TaskID` as the generated public identifier. Store the upstream identifier in `TaskPrivateData.UpstreamTaskID` and use it for provider requests only.
- Persist the selected channel key for every task platform. Polling and content retrieval use that snapshot instead of the channel's aggregate multi-key value.
- Route terminal task transitions through one CAS-protected path. Settlement/refund runs only after the caller wins the transition; failed refund claims are recoverable and cannot be processed twice.
- Real-time Gemini/Vertex fetch uses the same terminal transition path as background polling.
- Remove unconditional billing lifecycle bulk updates. Cache/adaptor lookup failures remain retryable unless a per-task CAS transition succeeds.
- Preserve configured model-ratio billing and add defaults for the newly ported Veo models.
- Resolve remix requests from the original task's upstream identifier.

## Error Handling

The existing adaptor response interface remains unchanged. Finalization errors are returned/logged with a durable task record whenever possible; no path should silently turn a failed charge into a successful task or an already settled task into a refund candidate.

## Testing

Add focused unit tests for public/upstream IDs, selected-key persistence, remix URL construction, ratio fallback/default prices, and CAS-gated terminal transitions. Run the full Go test suite, `go vet ./...`, frontend build, and `git diff --check`.
