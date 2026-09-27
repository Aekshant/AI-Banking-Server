# Development guide

## Prerequisites

| Tool | Version | Used for |
|---|---|---|
| Go | 1.27+ | The service and the tests |
| Docker + Compose | any recent | Postgres and Redis |
| Python | 3.8+ | Seeding synthetic data |
| [swag](https://github.com/swaggo/swag) | 1.16+ | Regenerating Swagger docs (optional) |

## First-time setup

Run from the repo root:

```bash
cp .env.example .env                  # set POSTGRES_PASSWORD
docker compose up -d                  # Postgres + Redis; schema created on first start

cd database/seed
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
.venv/bin/python seed.py              # 50 synthetic customers

cd ../..
make run                              # http://localhost:8001
```

Check it's up:

```bash
curl localhost:8001/health
# {"status":200,"message":"Service is healthy","success":true,"data":{"database":"up","redis":"up"}}
```

## Everyday commands

Run from the repo root:

| Command | What it does |
|---|---|
| `make run` | Start the service on `PORT` (default 8001) |
| `make test-unit` | Unit tests; no database needed |
| `make test-integration` | End-to-end tests against real Postgres and Redis |
| `make test-load` | Concurrent transfer load test |
| `make test-all` | Unit and integration tests |
| `make swagger` | Regenerate Swagger docs after changing handler annotations |
| `make seed` | Reset the 50 synthetic customers to their original values |
| `make up` / `make down` | Start or stop Postgres and Redis |
| `make up-app` | Run the service in a container instead of `make run` |
| `make up-observability` | Start Prometheus, Grafana, Loki, Tempo and Alloy |
| `make up-all` | Everything in Docker |
| `make logs` | Follow the service container's logs |
| `make image` | Build the service image only |

The Docker setup, observability stack and dashboards are documented in [infrastructure/README.md](../infrastructure/README.md).

## Configuration

Everything is read from environment variables. `.env` at the repo root is loaded automatically by Docker Compose, the service and the seed script (they search parent folders for it). Real environment variables take precedence over `.env`.

**Database**

| Variable | Required | Default | Notes |
|---|---|---|---|
| `POSTGRES_USER` | yes | | |
| `POSTGRES_PASSWORD` | yes | | Special characters are fine; they're escaped |
| `POSTGRES_DB` | yes | | |
| `POSTGRES_HOST` | | `localhost` | |
| `POSTGRES_PORT` | | `5432` | Also the host port Compose publishes |
| `POSTGRES_SSLMODE` | | `disable` | Use `require` or stricter outside local development |
| `DATABASE_URL` | | | Overrides all `POSTGRES_*` values |

**Redis and rate limiting**

| Variable | Default | Notes |
|---|---|---|
| `REDIS_HOST` | `localhost` | |
| `REDIS_PORT` | `6379` | |
| `REDIS_PASSWORD` | empty | |
| `RATE_LIMIT_ENABLED` | `true` | `false` removes the middleware entirely |
| `RATE_LIMIT_READ` | `30/10` | `capacity/refill_per_second` for profile reads |
| `RATE_LIMIT_TRANSFER` | `10/1` | Transfers |
| `RATE_LIMIT_LOAN` | `10/2` | Loan evaluation |
| `TRUSTED_PROXIES` | empty | Comma-separated IPs or CIDRs allowed to set `X-Forwarded-For`. Leave empty unless behind a proxy |

**Service**

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8001` | |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `json` | `text` is easier to read in a terminal |

**Observability**

| Variable | Default | Notes |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | unset (tracing off) | `http://localhost:4317` sends traces to Tempo from `make run`. The container sets it automatically |
| `OTEL_TRACES_SAMPLER` / `OTEL_TRACES_SAMPLER_ARG` | trace everything | e.g. `parentbased_traceidratio` / `0.1` for 10% |
| `GRAFANA_ADMIN_PASSWORD` | `admin` | Applied when Grafana first creates its data volume |

The service refuses to start if a required variable is missing or a rate limit is malformed, and the error names the variable.

## Testing

| Layer | Where | Needs | Command |
|---|---|---|---|
| Unit | Next to the code in `services/banking-service/internal/` | nothing | `make test-unit` |
| Integration | `tests/integration/` | Postgres + Redis running | `make test-integration` |
| Load | `tests/load/` | Postgres + Redis running | `make test-load` |

**Your data is safe.** Integration and load tests:

- start their own copy of the service on port `8099` (override with `TEST_PORT`), so they don't touch the server you're developing against;
- create their own customers (`itest-<run>-NNN`) and delete them, with their transactions, when done.

**Load test options:**

```bash
make test-load LOAD_ARGS="-workers 50 -requests 10000 -accounts 20 -retry-ratio 0.2"
```

It reports throughput, latency percentiles and status codes, and fails if the total money across its test accounts changes. Record notable results in [`benchmarks/`](benchmarks/).

See [`tests/README.md`](../tests/README.md) for what each test covers.

## Common tasks

**Add an endpoint**

1. Add the handler and repository method in the relevant `internal/<domain>/` package.
2. Add Swagger annotations to the handler, following the existing ones.
3. Register the route in that package's `RegisterRoutes`, and pass a rate limit rule in `cmd/main.go`.
4. Respond only through `internal/response` so the envelope stays consistent.
5. Add unit tests for any rules, and integration tests in `tests/integration/`.
6. Run `make swagger` and update [`api.md`](api.md).

**Change the schema**

Add a new numbered file in `database/migrations/` (for example `002_add_x.sql`). Postgres runs these scripts only when it creates a new, empty database, so apply the new file to an existing database by hand, or reset it with `docker compose down -v && docker compose up -d` (this deletes all data).

**Record a decision**

Copy [`decisions/template.md`](decisions/template.md) to the next number and link it from [`decisions/README.md`](decisions/README.md).

## Troubleshooting

| Symptom | Fix |
|---|---|
| `missing required environment variables: ...` | Create `.env` from `.env.example` at the repo root |
| `failed to connect to postgres` | Run `docker compose up -d` and check `docker compose ps` |
| `container name "/bank-postgres" is already in use` | An older Compose project owns the container: `docker rm -f bank-postgres bank-redis`, then `docker compose up -d` |
| Tables missing after `docker compose up` | The volume already existed, so migrations were skipped. Apply them by hand or reset the volume |
| `port 8099 is already in use` when testing | Stop whatever is on it, or run with `TEST_PORT=8097` |
| Profile returns `429` during manual testing | You exhausted a bucket. Wait for `Retry-After`, or set `RATE_LIMIT_ENABLED=false` locally |
| `make up-app` fails: port 8001 already allocated | `make run` is still running. Stop it; both use port 8001 |
| Grafana panels show "No data" | Send some requests first. Check http://localhost:9090/targets shows `banking-service` as up |
| No logs in Grafana's Logs panel | Logs reach Loki only when the service runs in a container (`make up-app`) |
| No traces from `make run` | Set `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317` in `.env` and start the observability profile |
| Changed `GRAFANA_ADMIN_PASSWORD` has no effect | Grafana stores it on first start. Remove the `grafana_data` volume to reset |
