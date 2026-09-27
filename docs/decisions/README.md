# Architecture Decision Records

Each record captures one significant decision: the context, what was decided, and the consequences. Records are numbered and never deleted. A reversed decision gets a new record that supersedes the old one.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-atomic-transfers-with-ordered-locking.md) | Atomic transfers with ordered locking | Accepted |
| [0002](0002-idempotency-keys.md) | Idempotency keys for transfers | Accepted |
| [0003](0003-deterministic-loan-rules.md) | Deterministic loan rules, no LLM | Accepted |
| [0004](0004-mask-pii-at-the-api.md) | Mask PII at the API layer | Accepted |
| [0005](0005-exact-decimal-money.md) | Exact decimal money, no floats | Accepted |
| [0006](0006-token-bucket-rate-limiting.md) | Token bucket rate limiting in Redis | Accepted |
| [0007](0007-observability-stack.md) | Observability with OpenTelemetry and the Grafana stack | Accepted |

To add one, copy [`template.md`](template.md) to the next number.
