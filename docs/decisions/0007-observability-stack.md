# 0007. Observability with OpenTelemetry and the Grafana stack

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

When an AI agent's transfer fails or is slow, we need to answer three questions quickly: *how often* is this happening (metrics), *where* in the request did it go wrong (traces), and *what* did the code report (logs). Three disconnected tools make that slow: you find a spike, then hunt through logs by timestamp, then guess which request it was.

## Decision

Instrument the service once, with open standards, and run a self-hosted Grafana stack where the three signals link to each other.

| Signal | In the service | Stored in |
|---|---|---|
| Metrics | Prometheus client, `/metrics` in OpenMetrics format | Prometheus |
| Traces | OpenTelemetry SDK, OTLP/gRPC export | Tempo |
| Logs | `log/slog` JSON to stdout | Loki, shipped by Grafana Alloy |

**The linking is the point:**

- Latency samples carry the trace id as an **exemplar**, so a slow point on a graph opens the exact trace.
- Every log line written during a request carries **`trace_id`** and **`request_id`**, stored as Loki structured metadata, so a trace opens its logs and a log line opens its trace.
- A client-supplied **`X-Request-ID`** (an agent's tool-call id) is echoed back and logged, so one agent call can be followed end to end.

**Choices within that:**

- **OpenTelemetry for traces**, not a vendor SDK, so the backend can change (Jaeger, Honeycomb, Datadog) by changing an environment variable. The standard `OTEL_*` variables configure it, and tracing is off unless an endpoint is set.
- **Metrics labelled by route template** (`/accounts/:id/profile`), never the raw path. Otherwise every customer id creates a new time series and Prometheus runs out of memory.
- **Business metrics, not just HTTP ones.** `transfers_total{outcome}` distinguishes "settled", "replayed by idempotency" and "insufficient funds", all of which are HTTP 2xx or 4xx but mean very different things.
- **No customer data in telemetry.** SQL spans record statement text but not parameter values; PAN and Aadhaar are never logged; customer ids never become metric labels.
- **Health checks are invisible to tracing and access logs.** Docker probes `/health` every 10 seconds; tracing them would bury real traffic.
- **Logs to stdout, shipped by Alloy.** The service doesn't know Loki exists. Alloy replaces Promtail, which Grafana has deprecated.
- **Everything behind a Compose profile**, so `docker compose up` for day-to-day work still starts only Postgres and Redis.

## Consequences

- One click from a latency spike to the slow SQL statement to the log line explaining it. Verified end to end: an exemplar's trace id resolved in Tempo to a 10-span transfer trace, and in Loki to that request's log lines.
- The stack adds seven containers. It's opt-in (`make up-observability`) for that reason.
- Every request is traced. At production volume this needs sampling (`OTEL_TRACES_SAMPLER_ARG`), and trace storage needs object storage instead of local disk.
- Alerts are evaluated by Prometheus but not delivered; Alertmanager is still to be added.
- `/metrics` is exposed on the API port, which is fine locally but should be restricted in production.
