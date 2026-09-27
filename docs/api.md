# API reference

Base URL: `http://localhost:8001`. Interactive docs are at [`/swagger/index.html`](http://localhost:8001/swagger/index.html) while the service is running.

| Method | Endpoint | Rate limit group |
|---|---|---|
| `GET` | [`/health`](#get-health) | none |
| `GET` | [`/metrics`](#get-metrics) | none |
| `GET` | [`/api/v1/accounts/{id}/profile`](#get-apiv1accountsidprofile) | read |
| `POST` | [`/api/v1/accounts/transfer`](#post-apiv1accountstransfer) | transfer |
| `POST` | [`/api/v1/loans/evaluate`](#post-apiv1loansevaluate) | loan |

## Conventions

### Response envelope

Every response, including errors, unknown routes (`404`) and wrong methods (`405`), has the same shape:

```json
{
  "status": 200,
  "message": "Profile fetched successfully",
  "success": true,
  "data": {}
}
```

| Field | Type | Notes |
|---|---|---|
| `status` | integer | Always equals the HTTP status code |
| `message` | string | Human-readable; safe to show to users |
| `success` | boolean | `true` for 2xx, `false` otherwise |
| `data` | object or `null` | The payload; `null` on most errors |

Internal errors never leak details. A `500` always says `"An unexpected error occurred on the server"`, and the cause is logged on the server.

### Request ids

Every response has an `X-Request-ID` header. Send your own (letters, digits, `.`, `_`, `-`, up to 128 characters), for example an agent's tool-call id, and it is echoed back and attached to every log line for that request. Otherwise one is generated. Quote it when reporting a problem.

Incoming W3C `traceparent` headers are honoured, so a trace started by the caller continues into this service.

### Money

- Amounts are JSON numbers with **at most 2 decimal places**, greater than zero, and at most 13 digits before the decimal point (they must fit `NUMERIC(15,2)`).
- `1.234`, `-5`, `0` and `1e3` are rejected with `400`, not rounded.
- Responses always show 2 decimals (`5000.00`), exactly as stored. No floating-point rounding happens anywhere.

### Rate limiting

Each client IP has a **token bucket** per endpoint group. A bucket holds up to *capacity* tokens and refills at a steady rate, and each request takes one token.

| Group | Capacity (burst) | Refill (sustained) | Setting |
|---|---|---|---|
| read | 30 | 10 per second | `RATE_LIMIT_READ=30/10` |
| transfer | 10 | 1 per second | `RATE_LIMIT_TRANSFER=10/1` |
| loan | 10 | 2 per second | `RATE_LIMIT_LOAN=10/2` |

Rate-limited responses include:

| Header | Meaning |
|---|---|
| `X-RateLimit-Limit` | Bucket capacity |
| `X-RateLimit-Remaining` | Whole tokens left after this request |
| `Retry-After` | On `429` only: seconds until a token is available |

When the bucket is empty:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
```
```json
{
  "status": 429,
  "message": "Too many requests, please retry later",
  "success": false,
  "data": { "retry_after_seconds": 1 }
}
```

If Redis is unavailable, requests are allowed (the limiter fails open). See [ADR 0006](decisions/0006-token-bucket-rate-limiting.md).

---

## `GET /health`

Checks Postgres and Redis. Not rate limited.

**`200`**: Postgres is up. Redis may be `"down"`, which is reported but not fatal, since only rate limiting depends on it.

```json
{
  "status": 200,
  "message": "Service is healthy",
  "success": true,
  "data": { "database": "up", "redis": "up" }
}
```

**`503`**: Postgres is down.

```json
{
  "status": 503,
  "message": "Database unavailable",
  "success": false,
  "data": { "database": "down", "redis": "up" }
}
```

---

## `GET /metrics`

Prometheus metrics in OpenMetrics text format. Not wrapped in the envelope and not rate limited. The metrics are listed in [infrastructure/README.md](../infrastructure/README.md#metrics-prometheus).

---

## `GET /api/v1/accounts/{id}/profile`

Returns a customer's profile. **PAN and Aadhaar are always masked**: the unmasked values never leave the service.

```bash
curl http://localhost:8001/api/v1/accounts/customer-001/profile
```

**`200`**

```json
{
  "status": 200,
  "message": "Profile fetched successfully",
  "success": true,
  "data": {
    "customer_id": "customer-001",
    "full_name": "Isaac Bakshi",
    "pan": "XXXXX2768C",
    "aadhaar": "XXXX XXXX 4428",
    "balance": 108145.55,
    "credit_score": 732,
    "is_2fa_verified": true
  }
}
```

| Field | Notes |
|---|---|
| `pan` | First 5 characters masked: `XXXXX1234F` |
| `aadhaar` | Only the last 4 digits shown: `XXXX XXXX 9012` |
| `balance` | Exact, 2 decimals |

| Status | Message |
|---|---|
| `404` | `Account not found` |
| `429` | Rate limited |

---

## `POST /api/v1/accounts/transfer`

Moves money between two accounts **atomically**: both balances change and the transaction is recorded, or nothing happens. **Idempotent**: re-sending a request with the same `idempotency_key` returns the original transaction instead of moving money again.

**Request**

| Field | Type | Rules |
|---|---|---|
| `from_account` | string | Required, max 64 characters |
| `to_account` | string | Required, max 64 characters, different from `from_account` |
| `amount` | number | Required, see [Money](#money) |
| `idempotency_key` | string | Required, max 128 characters. Generate a new one per logical transfer (a UUID works well) and reuse it for retries |

```bash
curl -X POST http://localhost:8001/api/v1/accounts/transfer \
  -H "Content-Type: application/json" \
  -d '{
    "from_account": "customer-001",
    "to_account": "customer-002",
    "amount": 5000,
    "idempotency_key": "tx-123"
  }'
```

**`201`**: transfer settled.

```json
{
  "status": 201,
  "message": "Transfer settled",
  "success": true,
  "data": {
    "transaction_id": "0884769a-809f-47e4-9b59-404da9ab249c",
    "status": "SETTLED",
    "amount": 5000.00
  }
}
```

**`200`**: this `idempotency_key` was already used for the **same** transfer. The original transaction is returned and no money moves.

```json
{
  "status": 200,
  "message": "Duplicate request; returning original transaction",
  "success": true,
  "data": {
    "transaction_id": "0884769a-809f-47e4-9b59-404da9ab249c",
    "status": "SETTLED",
    "amount": 5000.00
  }
}
```

**Errors**

| Status | Message | Cause |
|---|---|---|
| `400` | `Invalid request body: from_account, to_account, amount and idempotency_key are required` | Missing field, malformed JSON, or a field too long |
| `400` | `from_account and to_account must be different` | Same account on both sides |
| `400` | `amount must be a positive number with at most 2 decimal places` | Bad amount |
| `400` | `Insufficient funds` | Balance lower than `amount`. Nothing is recorded, so the same key can be retried later |
| `404` | `Account not found` | Either account doesn't exist |
| `409` | `idempotency_key was already used for a different transfer` | Key reused with different accounts or amount |
| `429` | Rate limited | |

**Retrying safely:** on a timeout or network error, re-send the **identical** request with the **same** key. You'll get either `201` (the first attempt never arrived) or `200` (it did), never a double transfer.

How this holds under concurrency: [ADR 0001](decisions/0001-atomic-transfers-with-ordered-locking.md), [ADR 0002](decisions/0002-idempotency-keys.md).

---

## `POST /api/v1/loans/evaluate`

Decides loan eligibility with **deterministic rules** (no LLM). The same inputs always give the same answer.

**Request**

| Field | Type | Rules |
|---|---|---|
| `customer_id` | string | Required, max 64 characters |
| `loan_amount` | number | Required, see [Money](#money) |

```bash
curl -X POST http://localhost:8001/api/v1/loans/evaluate \
  -H "Content-Type: application/json" \
  -d '{"customer_id": "customer-004", "loan_amount": 500000}'
```

**`200`**: returned for both approvals and rejections; check `approved`.

```json
{
  "status": 200,
  "message": "Loan evaluated",
  "success": true,
  "data": {
    "customer_id": "customer-004",
    "approved": true,
    "credit_score": 821,
    "reason": "ELIGIBLE",
    "max_eligible_amount": 938328.60
  }
}
```

**Rules**, checked in order, where the first failure decides `reason`:

| # | Rule | `reason` if it fails |
|---|---|---|
| 1 | Credit score ≥ 650 | `LOW_CREDIT_SCORE` |
| 2 | 2FA verified | `IDENTITY_NOT_VERIFIED` |
| 3 | `loan_amount` ≤ `max_eligible_amount` | `AMOUNT_EXCEEDS_LIMIT` |

If all pass, `reason` is `ELIGIBLE`.

`max_eligible_amount` = min(balance × multiplier, ₹1,00,00,000), where the multiplier is:

| Credit score | Multiplier |
|---|---|
| 750+ | 10× |
| 700–749 | 5× |
| 650–699 | 2× |

It is `0.00` when rule 1 or 2 fails. Use it to tell the customer how much they *can* borrow.

**Errors**

| Status | Message |
|---|---|
| `400` | `Invalid request body: customer_id and loan_amount are required` |
| `400` | `loan_amount must be a positive number with at most 2 decimal places` |
| `404` | `Account not found` |
| `429` | Rate limited |

Why rules instead of a model: [ADR 0003](decisions/0003-deterministic-loan-rules.md).
