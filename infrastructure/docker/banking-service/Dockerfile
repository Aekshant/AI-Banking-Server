# syntax=docker/dockerfile:1
#
# banking-service image. Build context: services/banking-service
#   docker build -f infrastructure/docker/banking-service/Dockerfile services/banking-service

# ---- build: static binary, no cgo ----
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X banking-service/internal/observability.Version=${VERSION}" \
      -o /out/banking-service ./cmd

# ---- runtime: distroless, no shell, non-root ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/banking-service /banking-service

ENV PORT=8001 LOG_FORMAT=json
EXPOSE 8001
USER nonroot:nonroot

# The binary checks its own /health; the image has no curl or shell.
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD ["/banking-service", "healthcheck"]

ENTRYPOINT ["/banking-service"]
