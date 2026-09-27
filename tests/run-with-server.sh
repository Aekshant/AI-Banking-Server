#!/usr/bin/env bash
# Builds banking-service, starts it on TEST_PORT (default 8099), runs the given
# command from the tests/ directory with BASE_URL pointing at it, then stops it.
#
# The server trusts X-Forwarded-For from localhost, as if behind a local proxy,
# so tests can act as different clients (and get separate rate-limit buckets).
#
#   ./run-with-server.sh go test ./integration/...
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT="${TEST_PORT:-8099}"
BIN_DIR="$ROOT/.bin"
mkdir -p "$BIN_DIR"

if curl -s -o /dev/null "http://localhost:$PORT/health"; then
  echo "port $PORT is already in use; set TEST_PORT to a free port" >&2
  exit 1
fi

(cd "$ROOT/services/banking-service" && go build -o "$BIN_DIR/banking-service" ./cmd)
(cd "$ROOT/services/banking-service" && PORT="$PORT" GIN_MODE=release TRUSTED_PROXIES="127.0.0.1,::1" exec "$BIN_DIR/banking-service" > "$BIN_DIR/server.log" 2>&1) &
PID=$!
trap 'kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do
  curl -sf -o /dev/null "http://localhost:$PORT/health" && break
  if ! kill -0 "$PID" 2>/dev/null; then
    echo "banking-service failed to start:" >&2
    cat "$BIN_DIR/server.log" >&2
    exit 1
  fi
  sleep 0.2
done

export BASE_URL="http://localhost:$PORT"
cd "$ROOT/tests"
"$@"
