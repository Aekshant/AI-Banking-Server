# Transfer load test with rate limiting: 2026-09-27

Same setup and command as the [baseline](2026-09-27-transfer-load.md), after adding Redis token-bucket rate limiting ([ADR 0006](../decisions/0006-token-bucket-rate-limiting.md)). Every request now makes one Redis round trip before reaching the handler.

The load generator sends each request from a random client IP, so the limiter runs on every request but never rejects. This measures the limiter's overhead, not its limits.

## Result

```
Transfer load test
  target       http://localhost:8099
  workers      20
  requests     2000 (10% retries of earlier requests)
  accounts     10
  duration     958ms
  throughput   2087 req/s
  latency      p50 8.7ms  p95 15.5ms  p99 21.6ms  max 38ms
  statuses     map[200:189 201:1811]
  errors       0
  money        10000000.00 before, 10000000.00 after
  result       PASS: every request settled or replayed, total money unchanged
```

## Compared with the baseline

| | Baseline | With rate limiting |
|---|---|---|
| Throughput | 2,382 req/s | 2,087 req/s |
| p50 | 7.6 ms | 8.7 ms |
| p99 | 17.2 ms | 21.6 ms |
| Errors | 0 | 0 |

Roughly a 1 ms median cost per request for the Redis check. Both numbers are single runs on a laptop where the load generator, service, Postgres and Redis share the CPU. Repeated runs would be needed before treating the difference as precise.
