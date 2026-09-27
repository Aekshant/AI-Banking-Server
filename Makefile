BANKING_SERVICE := services/banking-service
COMPOSE := docker compose

.PHONY: run test test-unit test-integration test-load test-all swagger seed \
        up up-app up-observability up-all down ps logs image

# ---- Docker (compose files live in infrastructure/) ----

up:                 ## Postgres + Redis
	$(COMPOSE) up -d

up-app:             ## + banking-service in a container (stop `make run` first: same port)
	$(COMPOSE) --profile app up -d --build

up-observability:   ## + Prometheus, Grafana, Loki, Tempo, Alloy, exporters
	$(COMPOSE) --profile observability up -d

up-all:             ## everything
	$(COMPOSE) --profile app --profile observability up -d --build

down:               ## stop everything (data volumes are kept)
	$(COMPOSE) --profile app --profile observability down

ps:
	$(COMPOSE) --profile app --profile observability ps

logs:               ## follow banking-service container logs
	$(COMPOSE) --profile app logs -f banking-service

image:              ## build the banking-service image only
	docker build -f infrastructure/docker/banking-service/Dockerfile -t bank-agent-platform/banking-service:dev $(BANKING_SERVICE)

# ---- Local development ----

run:
	cd $(BANKING_SERVICE) && go run ./cmd

test: test-unit

# Unit tests: fast, no database needed.
test-unit:
	cd $(BANKING_SERVICE) && go test ./...

# Integration tests: need Postgres (`docker compose up -d`). Starts the service automatically.
test-integration:
	./tests/run-with-server.sh go test -count=1 ./integration/...

# Load test: need Postgres. Override with e.g. `make test-load LOAD_ARGS="-workers 50 -requests 10000"`.
LOAD_ARGS ?= -workers 20 -requests 2000
test-load:
	./tests/run-with-server.sh go run ./load $(LOAD_ARGS)

test-all: test-unit test-integration

swagger:
	cd $(BANKING_SERVICE) && swag init -g cmd/main.go -o docs && swag fmt

seed:
	cd database/seed && .venv/bin/python seed.py
