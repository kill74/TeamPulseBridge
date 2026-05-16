# Quick Wins Implementation Plan

## Overview
This plan implements 5 high-impact, low-effort enhancements to the TeamPulseBridge ingestion-gateway service:

1. **Automatic retry mechanism** with exponential backoff for failed events
2. **Per-source rate limiting** extending the existing rate limiter
3. **Enhanced health checks** with dependency status monitoring
4. **Metrics dashboard** - Grafana templates for existing OTel metrics
5. **Event schema validation** layer before processing

---

## 1. Automatic Retry Mechanism with Exponential Backoff

### Current State
- Failed events are persisted to `failstore` (JSONL file)
- Manual replay via admin API (`/admin/events/replay`)
- No automatic retry logic exists

### Implementation

#### New File: `internal/retry/scheduler.go`
```go
package retry

type Scheduler struct {
    store       failstore.Store
    publisher   queue.Publisher
    logger      *slog.Logger
    maxRetries  int
    backoffFn   func(attempt int) time.Duration
    ticker      *time.Ticker
    stop        chan struct{}
    mu          sync.Mutex
    running     bool
}

// Key features:
// - Configurable max retries (default: 3)
// - Exponential backoff: 1s, 2s, 4s, 8s, 16s
// - Jitter to prevent thundering herd
// - Context-aware cancellation
// - Metrics tracking (retry_success, retry_failed)
// - Graceful shutdown
```

#### Changes to `internal/failstore/file_store.go`
- Add `RetryCount` and `NextRetryAt` fields to `FailedEvent`
- Add `ListDueForRetry(ctx, limit)` method to return events ready for retry
- Add `UpdateRetryStatus(ctx, eventID, status)` method

#### Changes to `cmd/server/main.go`
- Initialize retry scheduler with failed store and publisher
- Add scheduler to shutdown sequence
- Add config flags: `RETRY_ENABLED`, `RETRY_MAX_ATTEMPTS`, `RETRY_INTERVAL_SEC`

#### Config Additions
```go
RetryEnabled        bool
RetryMaxAttempts    int
RetryIntervalSec    int
```

### Testing Strategy
- Unit tests for backoff calculation with jitter
- Integration test: publish event -> fail -> auto-retry -> succeed
- Test graceful shutdown drains pending retries
- Test max retry limit enforcement

---

## 2. Per-Source Rate Limiting

### Current State
- Global rate limiter in `httpx.IPRateLimiter`
- Single RPM limit for all endpoints
- Admin has separate limit

### Implementation

#### Changes to `internal/httpx/middleware.go`
```go
type SourceRateLimitConfig struct {
    Enabled bool
    Sources map[string]int // source -> RPM limit
    Default int            // fallback RPM
}

// New middleware function:
func SourceRateLimit(cfg SourceRateLimitConfig) Middleware {
    // Creates per-source rate limiters
    // Uses source from URL path (/webhooks/slack -> "slack")
    // Falls back to default limit for unknown sources
}
```

#### Changes to `internal/config/config.go`
```go
SourceRateLimitEnabled bool
SourceRateLimits       map[string]int // e.g., {"slack": 100, "github": 200}
SourceRateLimitDefault int
```

#### Changes to `cmd/server/main.go`
- Parse source rate limit config from env (JSON format)
- Add middleware to chain after global rate limiter

### Testing Strategy
- Test per-source limits are enforced independently
- Test default limit applies to unknown sources
- Test middleware ordering (global -> source -> auth)
- Load test with mixed sources

---

## 3. Enhanced Health Checks with Dependency Status

### Current State
- `/healthz` returns static "ok"
- `/readyz` returns static "ready"
- No dependency health monitoring

### Implementation

#### Changes to `internal/handlers/health.go`
```go
type HealthChecker struct {
    publisher   queue.Publisher
    failStore   failstore.Store
    deduper     *dedup.Memory
    cfg         config.Config
}

func (h *HealthChecker) Healthz(w http.ResponseWriter, r *http.Request) {
    // Returns JSON with component statuses:
    // {
    //   "status": "healthy",
    //   "components": {
    //     "queue": {"status": "ok", "latency_ms": 2},
    //     "fail_store": {"status": "ok"},
    //     "dedup": {"status": "ok", "entries": 142}
    //   },
    //   "uptime_sec": 3600
    // }
}

func (h *HealthChecker) Readyz(w http.ResponseWriter, r *http.Request) {
    // Checks if all critical dependencies are available
    // Returns 503 if any critical dependency is down
    // Non-critical failures return 200 with warnings
}
```

#### Changes to `internal/queue/publisher.go`
- Add `HealthCheck(ctx) error` method to Publisher interface
- Implement for LogPublisher (always ok)
- Implement for PubSubPublisher (check connection)
- Implement for AsyncPublisher (check queue depth, failure ratio)

#### Changes to `cmd/server/main.go`
- Initialize HealthChecker with dependencies
- Replace simple health handlers with HealthChecker methods

### Testing Strategy
- Test healthy state returns 200
- Test queue failure returns appropriate status
- Test readyz vs healthz difference
- Test dependency timeout handling
- Test metrics exposure for health checks

---

## 4. Metrics Dashboard - Grafana Templates

### Current State
- OTel metrics already exposed via `/metrics`
- Counters: webhook_requests_total, security_rejections_total
- Queue metrics: buffer_usage_ratio, failure_budget_ratio, depth
- No visualization layer

### Implementation

#### New File: `deploy/grafana/dashboards/ingestion-gateway.json`
Grafana dashboard with panels for:

**Overview Panel**
- Request rate (req/s) by source
- Error rate (req/s) by source
- P50/P95/P99 latency
- Queue depth over time

**Reliability Panel**
- Success rate % over time
- Retry success/failure rates
- Failed event store size
- Dedup hit rate

**Queue Health Panel**
- Buffer usage ratio (gauge)
- Failure budget ratio (gauge)
- Backpressure events counter
- Publish success/failure ratio

**Security Panel**
- Rejections by reason
- Rate limit exceeded events
- Auth failures by source
- Security audit events

#### New File: `deploy/grafana/alerts/ingestion-gateway.yaml`
Alerting rules:
- Queue buffer > 80% for 5m
- Error rate > 5% for 5m
- Failed events > 100 in 1h
- Health check failure

### Testing Strategy
- Import dashboard to local Grafana
- Verify all metrics populate correctly
- Test alert triggers with simulated failures
- Document dashboard usage

---

## 5. Event Schema Validation Layer

### Current State
- No schema validation
- Raw JSON passed through to queue
- Invalid payloads only caught downstream

### Implementation

#### New File: `internal/schema/validator.go`
```go
package schema

type Validator struct {
    schemas map[string]*jsonschema.Schema
}

func (v *Validator) Validate(source string, body []byte) error {
    // Validates against source-specific schema
    // Returns detailed error messages
    // Metrics: schema_validation_pass, schema_validation_fail
}
```

#### New File: `internal/schema/schemas/slack.json`
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["type"],
  "properties": {
    "type": {"type": "string"},
    "challenge": {"type": "string"},
    "event_id": {"type": "string"}
  }
}
```

Similar schemas for: github.json, gitlab.json, teams.json

#### Changes to `internal/handlers/webhooks.go`
- Add validator to WebhookHandler
- Validate after signature check, before publish
- Return 400 with detailed error on validation failure
- Metrics tracking for validation outcomes

#### Changes to `internal/config/config.go`
```go
SchemaValidationEnabled bool
SchemaPath              string // path to schema files
```

### Testing Strategy
- Unit tests for each source schema
- Test valid payloads pass
- Test invalid payloads rejected with clear errors
- Test schema loading errors
- Performance test: validation overhead < 1ms

---

## Implementation Order & Dependencies

```
Phase 1 (Week 1):
├── 1. Enhanced Health Checks (foundational for monitoring)
└── 4. Metrics Dashboard (visualize existing + new metrics)

Phase 2 (Week 2):
├── 2. Per-Source Rate Limiting (builds on existing limiter)
└── 5. Event Schema Validation (independent feature)

Phase 3 (Week 3):
└── 1. Automatic Retry Mechanism (depends on health checks)
```

## Risk Assessment

| Feature | Risk Level | Mitigation |
|---------|-----------|------------|
| Retry Mechanism | Medium | Feature flag, max retries, circuit breaker |
| Per-Source Rate Limit | Low | Backward compatible, default fallback |
| Enhanced Health Checks | Low | Non-breaking, additive only |
| Metrics Dashboard | Zero | No code changes, config only |
| Schema Validation | Medium | Feature flag, graceful degradation |

## Configuration Changes

All new features are opt-in via environment variables:

```bash
# Retry
RETRY_ENABLED=false
RETRY_MAX_ATTEMPTS=3
RETRY_INTERVAL_SEC=1

# Per-Source Rate Limit
SOURCE_RATE_LIMIT_ENABLED=false
SOURCE_RATE_LIMITS={"slack": 100, "github": 200}
SOURCE_RATE_LIMIT_DEFAULT=150

# Schema Validation
SCHEMA_VALIDATION_ENABLED=false
SCHEMA_PATH=./schemas
```

## Testing Requirements

- All features must have unit tests
- Integration tests for retry mechanism
- Load tests for rate limiting
- Schema validation performance benchmarks
- Grafana dashboard import verification

## Rollout Strategy

1. Deploy with all features disabled
2. Enable health checks first (no risk)
3. Deploy Grafana dashboard
4. Enable per-source rate limiting in staging
5. Enable schema validation in staging
6. Enable retry mechanism with low max retries
7. Gradually increase retry attempts based on metrics
8. Full rollout to production
