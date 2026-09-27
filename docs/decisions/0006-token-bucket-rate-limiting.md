# 0006. Token bucket rate limiting in Redis

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

An AI agent calling this API can loop, retry aggressively or fan out requests. Without a limit, one misbehaving client can overload Postgres or hammer transfers. Legitimate clients also burst, for example an agent loading several profiles at once, so a hard "N per second" cap would reject normal traffic.

## Decision

Use a **token bucket** per client IP, with a separate bucket per endpoint group:

| Group | Endpoint | Default (capacity / refill per second) |
|---|---|---|
| `read` | `GET /accounts/{id}/profile` | 30 / 10 |
| `transfer` | `POST /accounts/transfer` | 10 / 1 |
| `loan` | `POST /loans/evaluate` | 10 / 2 |

A bucket holds up to *capacity* tokens and refills at a steady rate. Each request takes one token; an empty bucket means `429 Too Many Requests` with a `Retry-After` header. This allows bursts up to *capacity* while capping the sustained rate. Separate buckets mean heavy reading never uses up a client's transfer budget.

**Bucket state lives in Redis**, updated by one Lua script that refills and takes a token atomically. This means:

- Concurrent requests cannot both spend the last token.
- Limits hold across multiple service instances (an in-memory bucket would give each instance its own budget).
- Redis's clock (`TIME`) is used, so instances with drifting clocks still agree.
- Idle buckets expire once they would be full, so Redis memory stays bounded.

**Fail open.** If Redis is down or slower than 100 ms, the request is allowed and the error logged. The alternative, rejecting everything, would let a Redis outage take banking down. `/health` reports `"redis": "down"` so the degradation is visible.

**Client IP comes from the TCP connection**, not `X-Forwarded-For`, unless the proxy is listed in `TRUSTED_PROXIES`. Gin's default trusts the header from anyone, which would let a client claim a new IP on every request and bypass the limit.

Responses always carry `X-RateLimit-Limit` and `X-RateLimit-Remaining`.

## Consequences

- One Redis round trip per request. In the local load test, throughput went from 2,382 to 2,087 req/s (single runs on a laptop, so treat as indicative).
- While Redis is down there is **no** rate limiting. A deployment that must never go unprotected would pair this with a coarse in-memory limiter as a fallback.
- Limiting by IP is a stopgap: many customers behind one NAT share a bucket. Once authentication exists, buckets should be keyed by API key or customer.
- Limits are configured through `RATE_LIMIT_*` in `.env` without code changes.
