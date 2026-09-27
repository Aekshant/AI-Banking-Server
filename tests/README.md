# Tests

| Folder | What | Command |
|---|---|---|
| [`integration/`](integration/) | End-to-end: real HTTP requests to a running service, backed by real Postgres and Redis | `make test-integration` |
| [`load/`](load/) | Concurrent transfer load test that verifies no money is created or lost | `make test-load` |
| [`unit/`](unit/) | Placeholder. Go unit tests live next to the code (see below) | `make test-unit` |

`tests/` is its own Go module. Integration and load tests talk to the service only over HTTP (black-box) and use the database to set up data and check results.

## Running

Postgres and Redis must be running (`docker compose up -d` from the repo root). The make targets build the service, start it on port `8099` (override with `TEST_PORT`) and stop it afterwards.

Tests never touch your data: each one creates its own customers (`itest-<run>-NNN`) and deletes them, with their transactions, when it finishes. Each request comes from a random client IP, so tests never share a rate-limit bucket.

## Integration tests

| File | Covers |
|---|---|
| `health_test.go` | Health check; envelope on unknown routes (`404`) and wrong methods (`405`) |
| `accounts_test.go` | PAN and Aadhaar masked; raw PII never appears in the response; balance exact; `404` |
| `transfer_test.go` | Exact balances after settlement; retry returns the original; key reuse gives `409`; insufficient funds changes nothing; validation. Concurrency: 20 identical requests move money once; 100 opposite-direction transfers don't deadlock; 20 debits against ₹500 allow exactly 5 |
| `loans_test.go` | Every rule and boundary (one paisa over the limit, the ₹1 crore cap); `404`; validation |
| `ratelimit_test.go` | Bursting past capacity gives `429` with `Retry-After` while other clients are unaffected; buckets are separate per endpoint group |
| `observability_test.go` | `/metrics` exposes business metrics that move with transfers and never contains customer ids; `X-Request-ID` is echoed or generated |

## Load test

```bash
make test-load
make test-load LOAD_ARGS="-workers 50 -requests 10000 -accounts 20 -retry-ratio 0.2"
```

Sends random transfers between test accounts from many workers, re-sending a fraction of earlier requests to exercise idempotency. It reports throughput, p50/p95/p99 latency and status codes, and fails if the total balance changes. Results: [`docs/benchmarks/`](../docs/benchmarks/).

## Unit tests live next to the code

Go only lets a test reach a package's unexported code from the same folder, so unit tests sit in `services/banking-service/internal/`:

| Package | Covers |
|---|---|
| `accounts` | PAN and Aadhaar masking, including malformed input |
| `config` | Database URL built from `.env` with escaping; missing variables; rate-limit settings |
| `loans` | Every lending rule boundary |
| `money` | Amount validation |
| `observability` | Log lines carry request and trace ids; unsafe incoming request ids are replaced; metrics use route templates; latency exemplars carry the trace id; health checks produce unsampled contexts |
| `ratelimit` | The real Lua token bucket (burst, refill, capacity cap, separate clients, expiry) against an in-memory Redis with a controllable clock; middleware headers, `429` and fail-open behaviour |
| `response` | Envelope shape and the invalid-status fallback |
