# Benchmarks

Performance results, one file per run, named `YYYY-MM-DD-<what>.md`. Record the machine, the command and the raw output so results can be compared over time.

| Date | Benchmark | Result |
|---|---|---|
| 2026-09-27 | [Transfer load test](2026-09-27-transfer-load.md) | 2,382 req/s, p99 17.2 ms, money conserved |
| 2026-09-27 | [Transfer load test with rate limiting](2026-09-27-transfer-load-rate-limited.md) | 2,087 req/s, p99 21.6 ms, money conserved |

Run the load test with:

```bash
make test-load
make test-load LOAD_ARGS="-workers 50 -requests 10000 -accounts 20"
```
