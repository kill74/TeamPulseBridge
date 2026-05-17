# TeamPulseBridge — Scalability Architecture Plan

## Executive Summary

TeamPulseBridge is a well-architected Go webhook ingestion gateway with good foundational patterns (async queues, backpressure, health checks, K8s manifests, Argo Rollouts). However, **several critical bottlenecks will block horizontal scaling beyond a single instance**. This plan addresses all scalability gaps ranked by impact, with phased implementation.

---

## Current Architecture State

| Component | Current State | Scalability Limit |
|-----------|--------------|-------------------|
| Dedup | In-memory `map[string]time.Time` | Single instance only |
| Rate limiting | In-memory `map[string]rateWindow` | Per-instance, 3x limit with 3 instances |
| Failed store | Local JSONL file, `sync.Mutex` | Single writer, O(n) reads |
| Audit logs | Local JSONL file, full scan + rewrite | O(n) memory, catastrophic prune |
| Retry scheduler | Per-instance `sync.Map` | Duplicate retries across instances |
| Queue backend | GCP Pub/Sub + log (interface-based) | Well-designed, extensible |
| Observability | OTel + Prometheus + Grafana | Good foundation, missing histograms |
| K8s deployment | Argo Rollouts, HPA, PDB, security context | `readOnlyRootFilesystem` conflicts with file stores |

---

## Phase 1: P0 — Blockers for Horizontal Scaling

### 1.1 Distributed Deduplication (Redis)

**Problem:** In-memory dedup only works per-instance. Same event hitting different instances both get accepted.

**Solution:** Replace `dedup.Memory` with `dedup.Redis` using atomic SET + EXPIRE operations.

**Implementation:**
```go
// internal/dedup/redis.go
type Redis struct {
    client *redis.Client
    prefix string
    ttl    time.Duration
}

func (r *Redis) Seen(key string) bool {
    fullKey := fmt.Sprintf("%s:%s", r.prefix, key)
    // SETNX + EXPIRE atomically
    wasSet, err := r.client.SetNX(r.ctx, fullKey, "1", r.ttl).Result()
    if err != nil {
        // Fallback: allow event if Redis is down (fail-open)
        return false
    }
    return !wasSet // true = already seen (duplicate)
}
```

**Config changes:**
```go
DedupBackend string // "memory" | "redis"
DedupRedisURL string // "redis://localhost:6379/0"
```

**Interface compatibility:** The existing `dedupStore` interface (`Seen(key string) bool`) remains unchanged. Swap implementation via factory.

**Fallback behavior:** If Redis is unreachable, fail-open (allow event) rather than fail-closed (reject all). Log warning.

---

### 1.2 Distributed Rate Limiting (Redis or Ingress)

**Problem:** Per-instance rate limiting means N instances = N× the intended limit.

**Solution A — Redis-based (recommended for app-level control):**
```go
// internal/httpx/redis_limiter.go
type RedisRateLimiter struct {
    client *redis.Client
    prefix string
    window time.Duration
}

func (l *RedisRateLimiter) Allow(key string, limit int) bool {
    fullKey := fmt.Sprintf("%s:%s", l.prefix, key)
    // Atomic INCR + EXPIRE in Lua script
    script := `
        local current = redis.call('INCR', KEYS[1])
        if current == 1 then
            redis.call('EXPIRE', KEYS[1], ARGV[1])
        end
        return current
    `
    count, err := l.client.Eval(l.ctx, script, []string{fullKey}, int(l.window.Seconds())).Int()
    return err == nil && count <= limit
}
```

**Solution B — Ingress-level (more scalable, less app complexity):**
- Configure rate limiting in nginx/Envoy/API Gateway
- Remove app-level rate limiting entirely
- More accurate since it's before load balancer distribution

**Recommendation:** Implement Solution A (Redis) for now, document Solution B as the preferred approach at enterprise scale.

---

### 1.3 Fix K8s `readOnlyRootFilesystem` Conflict

**Problem:** Security context sets `readOnlyRootFilesystem: true` but file stores write to `data/` directory. No volume mount defined.

**Solution:** Add `emptyDir` volume for `data/` in K8s manifests:
```yaml
volumes:
  - name: data
    emptyDir:
      sizeLimit: 1Gi
volumeMounts:
  - name: data
    mountPath: /app/data
```

**For production:** Replace `emptyDir` with a PVC backed by persistent storage (EFS, Azure Files, etc.) for data durability across pod restarts.

---

## Phase 2: P1 — Critical for Production Scale

### 2.1 Migrate File Stores to PostgreSQL

**Problem:** JSONL files don't scale. Single writer mutex, O(n) reads, no concurrent access, catastrophic prune operation.

**Solution:** Keep existing `Store` interfaces, add `PostgresStore` implementations.

**Schema:**
```sql
-- Failed events
CREATE TABLE failed_events (
    event_id    TEXT PRIMARY KEY,
    source      TEXT NOT NULL,
    reason      TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    failed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    retry_count INT NOT NULL DEFAULT 0,
    headers     JSONB NOT NULL DEFAULT '{}',
    body        JSONB NOT NULL
);
CREATE INDEX idx_failed_events_failed_at ON failed_events(failed_at DESC);
CREATE INDEX idx_failed_events_source ON failed_events(source);

-- Replay audit
CREATE TABLE replay_audit (
    audit_id    TEXT PRIMARY KEY,
    event_id    TEXT NOT NULL,
    source      TEXT,
    actor       TEXT NOT NULL,
    mode        TEXT NOT NULL,
    result      TEXT NOT NULL,
    error_code  TEXT,
    http_status INT NOT NULL,
    request_id  TEXT,
    replayed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_replay_audit_replayed_at ON replay_audit(replayed_at DESC);
CREATE INDEX idx_replay_audit_actor ON replay_audit(actor);
CREATE INDEX idx_replay_audit_event_id ON replay_audit(event_id);

-- Security audit
CREATE TABLE security_audit (
    audit_id    TEXT PRIMARY KEY,
    category    TEXT NOT NULL,
    outcome     TEXT NOT NULL,
    source      TEXT NOT NULL,
    reason      TEXT NOT NULL,
    path        TEXT NOT NULL,
    http_status INT NOT NULL,
    request_id  TEXT,
    actor       TEXT,
    client_ip   TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_security_audit_occurred_at ON security_audit(occurred_at DESC);
-- Partition by month for automatic TTL
-- PARTITION BY RANGE (occurred_at)
```

**Config changes:**
```go
StoreBackend string // "file" | "sqlite" | "postgres"
PostgresDSN  string // "postgres://user:pass@host:5432/teampulse?sslmode=require"
```

**Migration path:**
1. **Phase 2a — SQLite** (quick win, zero infrastructure): Use `modernc.org/sqlite` (pure Go, no CGO). WAL mode for concurrent reads. Good for single-instance deployments.
2. **Phase 2b — PostgreSQL** (multi-instance): Full production deployment with connection pooling via `pgxpool`.

**Interface compatibility:** All existing `Store` interfaces remain unchanged. Factory pattern selects implementation based on config.

---

### 2.2 Circuit Breaker for Publisher

**Problem:** If Pub/Sub goes down, every publish blocks for 5s timeout. No fast-fail, no recovery signal.

**Solution:** Wrap `Publisher.Publish` with circuit breaker pattern.

**Implementation using `sony/gobreaker`:**
```go
// internal/queue/circuit_breaker.go
type CircuitBreakerPublisher struct {
    wrapped  Publisher
    breaker  *gobreaker.CircuitBreaker
    logger   *slog.Logger
}

func (p *CircuitBreakerPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
    _, err := p.breaker.Execute(func() (interface{}, error) {
        return nil, p.wrapped.Publish(ctx, source, body, headers)
    })
    if err != nil {
        if errors.Is(err, gobreaker.ErrOpenState) {
            return ErrCircuitOpen
        }
        return err
    }
    return nil
}
```

**Configuration:**
```go
CircuitBreakerMaxRequests uint32 // 10 (requests before half-open)
CircuitBreakerInterval    time.Duration // 30s (window for failure rate)
CircuitBreakerTimeout     time.Duration // 60s (open state duration)
CircuitBreakerFailureRate float64 // 0.5 (50% failure rate triggers open)
```

**Behavior:**
- **Closed:** Normal operation, track failures
- **Open:** Fast-fail with `ErrCircuitOpen` → HTTP 503, no queue attempt
- **Half-open:** Allow limited requests through, if successful → closed, if failed → open again

---

### 2.3 Retry Scheduler Leader Election

**Problem:** Multiple instances running retry schedulers will race to retry the same events from the same file.

**Solution:** Only one instance runs the retry scheduler at a time.

**Implementation:**
```go
// internal/retry/leader.go
type LeaderElection struct {
    client *redis.Client
    key    string
    ttl    time.Duration
    id     string // unique instance ID
}

func (l *LeaderElection) IsLeader(ctx context.Context) bool {
    // SETNX with TTL — only one instance succeeds
    wasSet, err := l.client.SetNX(ctx, l.key, l.id, l.ttl).Result()
    return err == nil && wasSet
}

func (l *LeaderElection) Renew(ctx context.Context) bool {
    // Extend TTL if we're still the leader
    script := `
        if redis.call('GET', KEYS[1]) == ARGV[1] then
            return redis.call('EXPIRE', KEYS[1], ARGV[2])
        end
        return 0
    `
    result, _ := l.client.Eval(ctx, script, []string{l.key}, l.id, int(l.ttl.Seconds())).Int()
    return result == 1
}
```

**Usage in scheduler:**
```go
func (s *Scheduler) Start(ctx context.Context) {
    go func() {
        ticker := time.NewTicker(s.interval)
        defer ticker.Stop()
        for {
            select {
            case <-ticker.C:
                if !s.leader.IsLeader(ctx) {
                    continue // Not the leader, skip
                }
                s.processRetries()
                s.leader.Renew(ctx) // Extend leadership TTL
            case <-s.stop:
                return
            }
        }
    }()
}
```

---

### 2.4 Histogram Metrics for Latency SLOs

**Problem:** Only counters and gauges exist. No P50/P95/P99 latency tracking.

**Solution:** Add histogram metrics for HTTP request duration and queue publish latency.

**Implementation:**
```go
// internal/observability/telemetry.go
HTTPRequestDuration = metric.Float64Histogram(
    "http_request_duration_seconds",
    metric.WithUnit("s"),
    metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
)

QueuePublishDuration = metric.Float64Histogram(
    "queue_publish_duration_seconds",
    metric.WithUnit("s"),
    metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1),
)
```

**Grafana dashboard updates:** Add panels for P50/P95/P99 latency, error rate, and throughput.

---

## Phase 3: P2 — Important Improvements

### 3.1 API Versioning

**Problem:** No versioning. Any change to request/response format breaks existing clients.

**Solution:** Add `/api/v1/` prefix to all routes. Use content negotiation for admin APIs.

**Implementation:**
```go
// cmd/server/main.go
webhookMux.HandleFunc("POST /api/v1/webhooks/slack", h.HandleSlack)
webhookMux.HandleFunc("POST /api/v1/webhooks/github", h.HandleGitHub)
webhookMux.HandleFunc("POST /api/v1/webhooks/gitlab", h.HandleGitLab)
webhookMux.HandleFunc("POST /api/v1/webhooks/teams", h.HandleTeams)

// Keep legacy routes for backward compatibility (deprecated)
webhookMux.HandleFunc("POST /webhooks/slack", h.HandleSlack) // DEPRECATED
```

**Deprecation strategy:**
- v1 routes are canonical
- Legacy routes return `Deprecation: true` header
- Sunset date documented in response headers
- Remove legacy routes in v2

---

### 3.2 HPA Custom Metric (Queue Depth)

**Problem:** HPA only scales on CPU. Queue depth is a better scaling signal for a message ingestion service.

**Solution:** Export `queue_buffer_depth` as a custom metric for HPA.

**K8s HPA update:**
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ingestion-gateway
spec:
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Pods
      pods:
        metric:
          name: queue_buffer_depth
        target:
          type: AverageValue
          averageValue: "2000"
```

**Requires:** Prometheus Adapter or KEDA to expose custom metrics to HPA.

---

### 3.3 K8s Best Practices

**Additions to K8s manifests:**

1. **Startup probe** (faster, safer pod startup):
```yaml
startupProbe:
  httpGet:
    path: /healthz
    port: 8080
  failureThreshold: 30
  periodSeconds: 10
```

2. **Pod anti-affinity** (spread across nodes):
```yaml
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app: ingestion-gateway
          topologyKey: kubernetes.io/hostname
```

3. **Topology spread constraints** (even distribution):
```yaml
topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule
    labelSelector:
      matchLabels:
        app: ingestion-gateway
```

4. **Resource limits tuning** (production-ready):
```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: "2"
    memory: 2Gi
```

---

### 3.4 Config Hot-Reload / Feature Flags

**Problem:** Changing rate limits, toggling dedup, or adjusting queue buffer requires a full restart.

**Solution A — File watcher (short term):**
```go
// internal/config/watcher.go
func Watch(path string, onChange func(Config)) error {
    watcher, _ := fsnotify.NewWatcher()
    watcher.Add(path)
    go func() {
        for range watcher.Events {
            cfg := LoadFromFile(path)
            onChange(cfg)
        }
    }()
    return nil
}
```

**Solution B — Feature flags via admin API (medium term):**
```go
// POST /admin/flags
type FlagUpdate struct {
    Flag    string `json:"flag"`
    Enabled bool   `json:"enabled"`
}

// Flags that can be toggled at runtime:
// - dedup.enabled
// - rate_limit.enabled
// - schema_validation.enabled
// - retry.enabled
// - source_rate_limit.enabled
```

**Recommendation:** Implement Solution B first (simpler, no external dependencies), add Solution A later for full config reload.

---

### 3.5 Trace Propagation to Pub/Sub Messages

**Problem:** Traces don't propagate through the queue. Can't trace a webhook from ingestion → queue → consumer.

**Solution:** Inject W3C traceparent header into Pub/Sub message attributes.

**Implementation:**
```go
// internal/queue/pubsub_publisher.go
func (p *PubSubPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
    // Extract trace context from incoming request
    traceparent := propagation.TraceContext{}.Inject(ctx)
    
    msg := &pubsub.Message{
        Data: payload,
        Attributes: map[string]string{
            "source":         source,
            "traceparent":    traceparent, // W3C trace Context
        },
    }
    // ...
}
```

**Consumer side:** Extract traceparent from message attributes and continue the trace.

---

### 3.6 Load Testing Suite

**Problem:** No load/stress tests. No capacity planning data.

**Solution:** Add k6 load testing scripts.

**Test scenarios:**
```javascript
// load-tests/webhook-throughput.js
export const options = {
    stages: [
        { duration: '2m', target: 100 },   // Ramp up to 100 RPS
        { duration: '5m', target: 100 },   // Sustained 100 RPS
        { duration: '2m', target: 500 },   // Spike to 500 RPS
        { duration: '5m', target: 500 },   // Sustained 500 RPS
        { duration: '2m', target: 0 },     // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(95)<200'],  // 95% of requests < 200ms
        http_req_failed: ['rate<0.01'],    // < 1% error rate
    },
};
```

**Makefile targets:**
```makefile
load-test:
    k6 run load-tests/webhook-throughput.js

load-test-stress:
    k6 run load-tests/stress-test.js

load-test-soak:
    k6 run load-tests/soak-test.js --duration 1h
```

---

## Phase 4: P3 — Nice to Have

### 4.1 Bulkhead Pattern (Per-Source Queue Isolation)

**Problem:** All sources share the same queue. A flood from one source can starve others.

**Solution:** Separate queues per source with independent backpressure.

**Implementation:**
```go
// internal/queue/bulkhead.go
type BulkheadPublisher struct {
    sources map[string]*SourcePublisher
    default *SourcePublisher
}

type SourcePublisher struct {
    queue    chan PublishRequest
    backpressure BackpressureMonitor
}
```

Each source gets its own queue channel and backpressure monitor. A flood from GitHub won't affect Slack webhook processing.

---

### 4.2 Additional Queue Backends

**Current:** Only GCP Pub/Sub + log.

**Add:**
- **Kafka** (high-throughput, ordered partitions)
- **AWS SQS** (managed, simple)
- **Redis Streams** (lightweight, self-hosted)

Each backend implements the same `Publisher` interface. Config-driven selection.

---

### 4.3 Vertical Pod Autoscaler (VPA)

**Problem:** Resource requests may drift from actual usage over time.

**Solution:** Deploy VPA in `Off` mode to monitor and recommend right-sized resources.

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: ingestion-gateway-vpa
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ingestion-gateway
  updatePolicy:
    updateMode: "Off"  # Recommendations only, no auto-update
```

---

### 4.4 Structured JSON Logging

**Problem:** stdout logs only, no JSON structured logging for log aggregation.

**Solution:** Add JSON log handler option.

**Implementation:**
```go
// internal/observability/logger.go
func NewLogger() *slog.Logger {
    if os.Getenv("LOG_FORMAT") == "json" {
        handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
            Level: slog.LevelInfo,
        })
        return slog.New(handler)
    }
    // default: text handler
    handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    return slog.New(handler)
}
```

---

## Implementation Order & Dependencies

```
Phase 1 (P0)
├── 1.3 Fix K8s readOnlyRootFilesystem ← Independent, quick win
├── 1.1 Distributed Dedup (Redis) ← Requires Redis dependency
└── 1.2 Distributed Rate Limiting (Redis) ← Can share Redis with dedup

Phase 2 (P1)
├── 2.1 Migrate to PostgreSQL ← Independent, can use SQLite first
├── 2.2 Circuit Breaker ← Independent
├── 2.3 Retry Leader Election ← Requires Redis (shares with 1.1/1.2)
└── 2.4 Histogram Metrics ← Independent

Phase 3 (P2)
├── 3.1 API Versioning ← Independent
├── 3.2 HPA Custom Metric ← Requires 2.4 (metrics)
├── 3.3 K8s Best Practices ← Independent
├── 3.4 Config Hot-Reload ← Independent
├── 3.5 Trace Propagation ← Independent
└── 3.6 Load Testing ← Independent

Phase 4 (P3)
├── 4.1 Bulkhead Pattern ← Independent
├── 4.2 Additional Queue Backends ← Independent
├── 4.3 VPA ← Independent
└── 4.4 Structured Logging ← Independent
```

---

## Infrastructure Dependencies

| Component | Purpose | Alternatives |
|-----------|---------|-------------|
| **Redis** | Dedup, rate limiting, leader election | Memcached (no atomic ops), etcd (heavier) |
| **PostgreSQL** | Persistent store for failed events, audit logs | MySQL, SQLite (single-instance), DynamoDB |
| **Prometheus Adapter** | Custom metrics for HPA | KEDA (simpler, supports more triggers) |
| **k6** | Load testing | vegeta, wrk2, Locust |

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Redis dependency adds operational complexity | Medium | Medium | Use managed Redis (Cloud Memorystore, ElastiCache), implement graceful degradation |
| PostgreSQL migration breaks existing data | Low | High | Keep file store as fallback, dual-write during migration, data migration script |
| Circuit breaker causes false positives | Low | Medium | Conservative thresholds, monitoring, easy config override |
| API versioning increases maintenance burden | Medium | Low | Automated deprecation warnings, clear sunset policy |
| Load tests reveal capacity issues | High | Positive | Better to find issues before production traffic |

---

## Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Max instances | 1 | 10+ | HPA scaling events |
| Dedup accuracy (multi-instance) | 0% (broken) | 99.9% | Duplicate event rate in metrics |
| Rate limit accuracy (multi-instance) | 1/N of target | 95-105% of target | Rate limit rejection rate |
| P99 latency | Unknown | < 200ms | Histogram metrics |
| Queue publish success rate | Unknown | > 99.9% | Circuit breaker state + error rate |
| Time to detect queue backend failure | 5s (timeout) | < 1s | Circuit breaker open state |
| Data durability | File-dependent | 99.99% | PostgreSQL replication + backups |
