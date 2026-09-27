# Transfer load test: 2026-09-27

## Setup

| | |
|---|---|
| Machine | Intel Core i5-1135G7 (8 threads), 15 GB RAM, laptop |
| Database | PostgreSQL 16 in Docker, same machine |
| Service | banking-service, `GIN_MODE=release`, same machine |
| Command | `make test-load` (`-workers 20 -requests 2000 -accounts 10 -retry-ratio 0.1`) |

The load generator creates 10 test accounts with ₹10,00,000 each, then sends random transfers of ₹1–500 between random pairs from 20 concurrent workers. 10% of requests re-send an earlier request with the same idempotency key, to exercise the retry path under load.

## Result

```
Transfer load test
  target       http://localhost:8099
  workers      20
  requests     2000 (10% retries of earlier requests)
  accounts     10
  duration     840ms
  throughput   2382 req/s
  latency      p50 7.6ms  p95 13.6ms  p99 17.2ms  max 25.2ms
  statuses     map[200:198 201:1802]
  errors       0
  money        10000000.00 before, 10000000.00 after
  result       PASS: every request settled or replayed, total money unchanged
```

## Observations

- **Correctness held under load.** Every request either settled (`201`) or was recognised as a retry (`200`). No deadlocks, no errors, and the total money across all accounts was unchanged.
- **Retries were caught.** 198 requests were re-sends of an earlier request (each request has a 10% chance of being one). All 198 returned the original transaction with `200` and moved no money.
- **Contention is the limit.** With only 10 accounts, many transfers compete for the same row locks (ADR 0001). Spreading the load across more accounts should raise throughput. Worth measuring next.

## Caveats

Everything ran on one laptop, so the load generator, service and database competed for the same CPU. Treat the numbers as a baseline for comparing future changes, not as production capacity.
