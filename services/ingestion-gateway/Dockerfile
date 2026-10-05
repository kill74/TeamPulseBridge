# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/ingestion-gateway ./cmd/server \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/eventstore ./cmd/eventstore

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/ingestion-gateway /app/ingestion-gateway
COPY --from=builder /out/eventstore /app/eventstore
COPY --from=builder /src/internal/schema/schemas /app/internal/schema/schemas

# Runtime performance defaults: GOMEMLIMIT + GOMAXPROCS aware via cgroups,
# single-stack traces to keep crash logs small.
# NOTE: no HEALTHCHECK here (distroless has no shell/wget); orchestrate via
# Kubernetes liveness/readiness probes on /healthz and /readyz instead.
ENV GOTRACEBACK=single \
    GOMEMLIMIT=512MiB \
    GOMAXPROCS=4
EXPOSE 8080
USER nonroot:nonroot
# Exec form preserves signals for graceful 30s drain.
ENTRYPOINT ["/app/ingestion-gateway"]
