# Performance Guide — Ingestion Gateway

Senior-engineer tuning notes for making the gateway faster and more responsive.
All defaults below are already applied; this doc explains _why_ and how to verify.

## Raw-speed track (v2 — single RTT, zero-copy, prepared PG, H2C)

Opt-in single RTT gate (`RATE_LIMIT_COMBINED=true`, Redis only):

- `internal/httpx/redis_rate_limiter.go:CombinedCheckWithContext` runs
  `INCR general + INCR source + SETNX dedup` in one `EVALSHA` (1 RTT, 50ms
  fail-open). `internal/httpx/combined.go:RateLimitAndDedupCombined` replaces
  `RateLimit+SourceRateLimit+handler-Seen` when enabled; header-only event IDs
  (`X-GitHub-Delivery`, `X-Gitlab-*`) short-circuit duplicates with 202.
  Body-derived IDs (slack/teams) still use handler dedup. Default `false` for
  backward compat; enable after Redis testing (`combined_test.go` covers
  fail-open + path parsing).
- `redis.UniversalClient` (`dedup/redis.go`, `httpx/redis_rate_limiter.go`) adds
  `REDIS_CLUSTER_ADDRS` support; `cmd/server/main.go` builds single or cluster
  clients with `PoolSize 32/MinIdle 8/100ms/MaxRetries 0`.

Zero-copy + singleflight:

- `queue/envelope.go:NewRawWebhookEnvelopeNoCopy` saves 1×1MiB copy on the
  Pub/Sub worker (async already owns its clone). `pubsub_publisher.go` uses it
  and sets `body_len/event_id/body_hash (≤256KiB)` attrs.
- `eventstore/record.go:BuildRecord` references `envelope.Body` directly (no
  `append(nil)`), honors `BodyHashHint/Attrs["body_hash"]`; `consumer.go` passes
  through `msg.Attributes`. `providerEventID` also checks `X-Event-ID/event_id`.
- `handlers/health.go`: 2s memoize + `singleflight` coalesces kube+prom stampedes;
  `handlers/admin.go:FailedEvents` has 2s memoize + weak `ETag` (`If-None-Match`
  → 304) with `Cache-Control: private, max-age=2`.

Postgres prepared + bulk:

- Gateway pool sets `statement_cache_mode=prepare` (`cmd/server/main.go`);
  `eventstore/postgres_store.go:CopyFromEvents` stages via `COPY` to temp table
  then `INSERT ... ON CONFLICT DO NOTHING` for backfills >500 rows (live path
  keeps `pgx.Batch`). New composite indexes in `failstore/replayaudit/
  securityaudit/postgres_store.go`; offline `CONCURRENTLY` SQL in each
  `*/migrations/002_perf_indexes.sql`.

HTTP/2 + K8s alignment:

- `HTTP_H2C_ENABLED=true` wraps mux in `h2c` (`cmd/server/h2c.go`,
  `MaxConcurrentStreams 250`) for mesh multiplexing; edge stays HTTP/1.
- `deploy/k8s/base/configmap.yaml` aligned to code defaults (`WORKERS 16`,
  `GOROUTINES 16`, `BATCH 20/500`, `CB 25/8s`, `POOL 32`, `INFLIGHT 512`,
  `OTEL 0.02`); `base/rollout.yaml` `500m/512Mi → 2/1536Mi` fits 512 inflight;
  prod overlay `WORKERS 16/GOROUTINES 16/RETRY_AFTER 2s`.

Verify raw-speed:

```bash
RATE_LIMIT_COMBINED=true REDIS_ADDR=... go run ./cmd/server
k6 run -e TARGET_URL=http://localhost:8080 ../../load-tests/webhook-all-sources.js
psql $DATABASE_URL -f internal/eventstore/migrations/002_perf_indexes.sql
h2load -n10000 -c50 -m10 http://localhost:8080/healthz  # with HTTP_H2C_ENABLED=true
```

## What was slow (before — v1 track)

- `QUEUE_WORKERS=1` + blocking `PubSub.Publish.Get()` serialized all publishes (~5–20 rps).
- 3 serial Redis RTTs per webhook (2× rate-limit + dedup) with 250ms–2s budgets inflated p99.
- PII regex + structural scrub + 3× body copies + 4–5 JSON scans ran on the request goroutine.
- `FileStore.Save` did `MkdirAll+Write+f.Sync()` (plus prune scan for security audit) under a global `Mutex` on error/reject paths.
- Event-store: 1 goroutine, 1 INSERT/row, `ON CONFLICT DO UPDATE` (WAL churn), 4 indexes, `Info` per row, tight `Nack` loop.
- Middleware: full chain on probes, raw `path` label cardinality, `slog JSON+Callers` per request, crypto trace IDs on 100% traffic.
- Server: `ReadHeader 5s`, no `MaxHeaderBytes`/inflight cap, `Shutdown 10s` truncated drains.

## What changed (after)

### Request hot path (`internal/handlers/webhooks.go`, `platform/signature`)
- Pooled 32KiB→1MiB body buffers (`sync.Pool`), `CopyN(LimitReader, 1MiB+1)` overflow detect, `Close` always.
- Slack HMAC writes `v0:`/`ts`/`:`/body incrementally; `hmac.Equal` on raw bytes (no hex-string compare).
- Single JSON decode for Slack challenge+event_id; streaming `extractJSONField` with `bytes.Contains` fast-path.
- `fallbackEventID` hashes full body ≤32KiB, else prefix+len (avoids SHA256 over 1MiB).
- Static `acceptedResponse` struct (no per-request maps), `Retry-After` on both throttled (429) and full (503).
- Dedup `Forget` only on retriable enqueue failures.

### Dedup + rate limiting (`dedup/`, `httpx/redis_rate_limiter.go`, `httpx/middleware.go`)
- Memory dedup: 32-shard `RWMutex`, read-first fast path, 200k cap, background expiry.
- Redis dedup/rate-limit: 100ms/50ms budgets, fail-open, `FNV-1a` keys, ctx-aware (`SeenWithContext`, `AllowWithContext`).
- `IPRateLimiter`: 16 shards, per-shard caps, background sweep (no full-map scan in hot path).
- CIDR parse cached (`sync.Map`), `ClientIP` fast-path when no trusted proxies.
- Probes (`/healthz|/readyz|/metrics|/assets/*`) bypass timeout + rate limits.
- `RouteTemplate` (`/webhooks/:source`, `/admin/:endpoint`) bounds metric cardinality.
- `AccessLog`: `Debug` on success, `Info` on ≥400/slow; `MaxInflight(512)` fail-fast 503+`Retry-After`.
- `RequestID`: counter-based IDs, lazy traceparent (no crypto on unsampled traffic), sanitize only when dirty.
- JWT: pre-derived secret, reused `jwt.Parser`, strict `Bearer ` (no `ToLower` alloc).
- Recorder supports `Flusher`/`Hijacker`; middleware order fixed (Recoverer outermost → RequestID/OTEL → AccessLog → inflight/timeout → limits/auth).

### Queue (`internal/queue/`)
- `AsyncPublisher`: default workers 16, lock-free depth pre-check, single `Snapshot`, 10s worker budget, stale-deadline floor (1s), readiness fails only at 100% full / 25% failures.
- `PubSubPublisher`: bundling `20ms/500/1MiB`, `NumGoroutines 16`, flow-control `signal_error/2000/100MiB`, 1MiB guard, health cached 10s/2s.
- `Bulkhead`: enabled by default, allowlist (`slack|github|gitlab|teams` + `other`), `maxSources 8`, no double `Snapshot`, `Close` outside lock, 5s aggregate cache.
- `CircuitBreaker`: threshold 25/recovery 8s (env-tuned), single-flight half-open probe, ignores `ErrQueueFull/Throttled`.
- `Factory`: PII scrub moved **inside workers** (`SizeGatedScrubPublisher`, `PII_SCRUB_ASYNC=true`, `PII_MAX_SCRUB_BYTES=256KiB`), memory-bound clamp, combined close errors.
- Scrub fast-paths: `ScrubEmails` skips without `@`, `ScrubTokens` skips without `=`/`:`; `StructuralScrubber` uses `map` set + `containsFold` + 4KiB hint scan.

### Persistence (`failstore/`, `replayaudit/`, `securityaudit/`, `eventstore/`, `retry/`)
- New `AsyncStore` wrappers (512/1024 buffers, 300–500ms budgets, drop counters) so rejects/replays return immediately.
- `FileStore`: `MkdirAll` once, `RWMutex` (reads concurrent), limit caps; security audit prunes in background, not per-`Save`.
- `PostgresStore (eventstore)`: `ON CONFLICT DO NOTHING` + `SELECT` fallback, dropped `body_hash` index, new `SaveBatch` (`pgx.Batch`).
- `Consumer`: poison-pill Ack after 5 deliveries, backoff sleep before `Nack`, sampled logging (Debug + Info every 100).
- `cmd/eventstore`: `2000 msgs/256MiB/16 goroutines/10m extension`, pg pool `32/8`, statement cache, client retry with backoff.
- `cmd/server` pg pool `20/2`, Redis pool `32/8/100ms/MaxRetries 0`, retry workers 8 + jittered ticks + parallel retries, leader TTL `3×interval`.
- `Healthz`: fan-out with 1.5s overall budget (was 3s serial).
- `Admin`: batch replay with 5 workers (order-preserving), `bodyPreview` truncates before redact.
- `Schema`: precomputed `requiredSet`, small `seen` map (no full field-set alloc).
- `Telemetry`: 6 buckets (not 11/9), top-8 source cap, sampler `OTEL_TRACES_SAMPLER_RATIO=0.02`, batcher `2s/256`, probe spans named `probe`.
- `Logger`: `AddSource:false` in prod.
- Server: `ReadHeader 3s`, `MaxHeaderBytes 8KiB`, `Shutdown 30s`; compose limits + `GOMEMLIMIT/GOMAXPROCS`.

## Env knobs (see `.env.example`)

```
QUEUE_WORKERS=16  QUEUE_BULKHEAD_ENABLED=true  QUEUE_BULKHEAD_MAX_SOURCES=8
PUBSUB_PUBLISH_GOROUTINES=16  PUBSUB_BATCH_DELAY_MS=20  PUBSUB_FLOW_CONTROL_BEHAVIOR=signal_error
PII_SCRUB_ASYNC=true  PII_MAX_SCRUB_BYTES=262144
REDIS_POOL_SIZE=32  REDIS_IO_TIMEOUT_MS=100
HTTP_MAX_INFLIGHT=512  OTEL_TRACES_SAMPLER_RATIO=0.02
EVENTSTORE_MAX_OUTSTANDING_MESSAGES=2000  EVENTSTORE_RECEIVE_GOROUTINES=16
```

## Verify like a senior

```bash
cd services/ingestion-gateway
gofmt -w ./cmd ./internal && go vet ./... && go test ./... && go test -race ./...
go test -run=^$ -bench=. -benchmem -benchtime=5s ./internal/handlers ./internal/queue
k6 run -e TARGET_URL=http://localhost:8080 ../../load-tests/webhook-throughput.js
k6 run -e TARGET_URL=http://localhost:8080 ../../load-tests/webhook-all-sources.js
k6 run -e TARGET_URL=http://localhost:8080 -e ADMIN_JWT=$JWT ../../load-tests/admin-lists.js
```

Watch: `p95<200ms` at 200VU, queue depth, `queue_backpressure_events_total`,
`security_rejections_total{route}`, `pg_stat_activity`, Redis latency, no leader flap.

## Trade-offs accepted

- Fail-fast 429/503 + `Retry-After` over unbounded buffering (protects p99, clients retry).
- Async audit with tiny loss window on crash (drop counters expose it) over blocking rejects.
- Bulkhead `other` bucket over per-attacker-source queues (bounds goroutines).
- Local-first rollout: code defaults + compose first, then k8s HPA (`RPS/p95 + queue depth`), PDB, `requests/limits`.
