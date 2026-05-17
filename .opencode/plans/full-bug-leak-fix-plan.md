# TeamPulseBridge — Full Bug & Leak Fix Plan

## CRITICAL FIXES

### 1. Nil dereference in `checkQueue` — `handlers/health.go:112-132`

**Problem:** `checkQueue` calls `h.publisher.HealthCheck()` with no nil check. `Readyz` has the check, `Healthz` doesn't.

**Fix:** Add nil guard at the top of `checkQueue`:

```go
func (h *HealthChecker) checkQueue(ctx context.Context) componentHealth {
    start := time.Now()
    checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
    defer cancel()

    if h.publisher == nil {
        return componentHealth{
            Status: "disabled",
            Error:  "publisher not configured",
        }
    }

    err := h.publisher.HealthCheck(checkCtx)
    // ... rest unchanged
}
```

---

### 2. Unsafe type assertions — `retry/scheduler.go:112-113, 130-131`

**Problem:** `val.(int)` without ok-check will panic if non-int stored.

**Fix:** Use safe form at both locations:

```go
// Line 111-113 (processRetries)
if val, ok := s.retries.Load(event.EventID); ok {
    if n, ok := val.(int); ok {
        currentRetries = n
    }
}

// Line 129-131 (retryEvent)
if val, ok := s.retries.Load(event.EventID); ok {
    if n, ok := val.(int); ok {
        currentRetries = n
    }
}
```

---

### 3. Insecure RNG — `retry/scheduler.go:8,54`

**Problem:** `math/rand` with `time.Now().UnixNano()` seed is predictable.

**Fix:** Replace `math/rand` with `crypto/rand`:

```go
// Replace import
import "crypto/rand"  // remove "math/rand"

// Remove rng field from struct:
type Scheduler struct {
    // ... all fields except rng
    // REMOVE: rng *rand.Rand
}

// Remove rng initialization in NewScheduler:
return &Scheduler{
    // ... all fields except rng
    // REMOVE: rng: rand.New(rand.NewSource(time.Now().UnixNano())),
}

// Replace calculateBackoff jitter:
func (s *Scheduler) calculateBackoff(retryCount int, baseInterval time.Duration) time.Duration {
    if retryCount < 0 {
        retryCount = 0
    }
    exp := math.Pow(2, float64(retryCount))
    backoff := time.Duration(float64(baseInterval) * exp)

    jitterBytes := make([]byte, 8)
    if _, err := rand.Read(jitterBytes); err == nil {
        var jitterVal uint64
        for i := 0; i < 8; i++ {
            jitterVal = (jitterVal << 8) | uint64(jitterBytes[i])
        }
        jitter := time.Duration(float64(jitterVal) / float64(^uint64(0)) * float64(baseInterval))
        backoff += jitter
    }

    maxBackoff := 5 * time.Minute
    if backoff > maxBackoff {
        backoff = maxBackoff
    }
    return backoff
}
```

---

### 4. Goroutine leak in RateLimit middleware — `httpx/middleware.go:229-232, 276-279`

**Problem:** When `cfg.Limiter` is nil, a new `IPRateLimiter` is created (starts goroutine) but `Stop()` is never called.

**Fix:** Return a cleanup function from the middleware:

```go
func RateLimit(cfg RateLimitConfig) (Middleware, func()) {
    if !cfg.Enabled {
        return func(next http.Handler) http.Handler { return next }, func() {}
    }
    var internalLimiter *IPRateLimiter
    limiter := cfg.Limiter
    if limiter == nil {
        internalLimiter = NewIPRateLimiter(cfg.Now, cfg.Window, cfg.CleanupN)
        limiter = internalLimiter
    }
    // ... rest of middleware unchanged ...
    return middleware, func() {
        if internalLimiter != nil {
            internalLimiter.Stop()
        }
    }
}
```

**Caller update in `cmd/server/main.go`** — The `HandlerBuilder.Build()` already passes `b.limiter` so the leak doesn't occur in production. But for safety, add cleanup to `HandlerBuilder`:

```go
type HandlerBuilder struct {
    // ... existing fields
    limiterCleanup func()
}

func (b *HandlerBuilder) Stop() {
    b.limiter.Stop()
    if b.limiterCleanup != nil {
        b.limiterCleanup()
    }
}
```

**Simpler alternative:** Just document that `cfg.Limiter` is required and panic if nil:

```go
func RateLimit(cfg RateLimitConfig) Middleware {
    if !cfg.Enabled {
        return func(next http.Handler) http.Handler { return next }
    }
    if cfg.Limiter == nil {
        panic("httpx.RateLimit: cfg.Limiter is required to prevent goroutine leaks")
    }
    // ... rest unchanged, remove the nil fallback
}

func SourceRateLimit(cfg SourceRateLimitConfig) Middleware {
    if !cfg.Enabled || len(cfg.Sources) == 0 {
        return func(next http.Handler) http.Handler { return next }
    }
    if cfg.Limiter == nil {
        panic("httpx.SourceRateLimit: cfg.Limiter is required to prevent goroutine leaks")
    }
    // ... rest unchanged, remove the nil fallback
}
```

---

### 5. Double-close panic — `httpx/middleware.go:76-78` and `dedup/memory.go:55-57`

**Problem:** `Stop()` does `close(l.stop)` with no `sync.Once` guard.

**Fix for `IPRateLimiter`:**

```go
type IPRateLimiter struct {
    mu       sync.Mutex
    entries  map[string]rateWindow
    now      func() time.Time
    window   time.Duration
    windowS  int64
    hits     uint64
    cleanup  int
    stop     chan struct{}
    stopOnce sync.Once   // ADD THIS
}

func (l *IPRateLimiter) Stop() {
    l.stopOnce.Do(func() {
        close(l.stop)
    })
}
```

**Fix for `dedup.Memory`:**

```go
type Memory struct {
    enabled bool
    ttl     time.Duration
    mu      sync.Mutex
    seen    map[string]time.Time
    stop    chan struct{}
    stopOnce sync.Once   // ADD THIS
}

func (m *Memory) Stop() {
    m.stopOnce.Do(func() {
        close(m.stop)
    })
}
```

---

### 6. Division by zero — `httpx/middleware.go:68, 95-96, 113`

**Problem:** If `window < 1 second`, `windowS` becomes 0 → modulo by zero panic.

**Fix:** Validate minimum window in constructor:

```go
func NewIPRateLimiter(now func() time.Time, window time.Duration, cleanupEveryN int) *IPRateLimiter {
    if now == nil {
        now = time.Now
    }
    if window < time.Second {
        window = time.Minute
    }
    if cleanupEveryN <= 0 {
        cleanupEveryN = 1024
    }
    l := &IPRateLimiter{
        entries: make(map[string]rateWindow, 256),
        now:     now,
        window:  window,
        windowS: int64(window / time.Second),
        cleanup: cleanupEveryN,
        stop:    make(chan struct{}),
    }
    go l.periodicCleanup()
    return l
}
```

---

## HIGH SEVERITY FIXES

### 7. Hardcoded secrets — `.env:9`, `.env.example:9`

**Problem:** `ADMIN_JWT_SECRET=change-me` shipped in repo.

**Fix:** Replace with empty placeholder and add comment:

```
# .env
ADMIN_JWT_SECRET=  # Must be set to a strong secret (min 32 chars) in production

# .env.example
ADMIN_JWT_SECRET=  # Must be set to a strong secret (min 32 chars) in production
```

Also change defaults to fail-secure (see #23).

---

### 8. Insecure OTLP — `observability/telemetry.go:151`

**Problem:** `otlptracehttp.WithInsecure()` sends traces over HTTP.

**Fix:** Remove `WithInsecure()` and let the endpoint scheme determine security:

```go
func setupTracing(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, func(context.Context) error, error) {
    endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
    if endpoint == "" {
        tp := sdktrace.NewTracerProvider(
            sdktrace.WithResource(res),
            sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
        )
        return tp, tp.Shutdown, nil
    }

    exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint))
    if err != nil {
        return nil, nil, fmt.Errorf("init otlp trace exporter: %w", err)
    }
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithResource(res),
        sdktrace.WithBatcher(exporter),
        sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
    )
    return tp, tp.Shutdown, nil
}
```

Note: Also fixed sampling rate from 0.2 → 0.1 to match (see #22).

---

### 9. JWT ParseUnverified — `handlers/admin.go:812-819`

**Problem:** `jwt.ParseUnverified()` parses without signature verification — attacker can forge claims.

**Fix:** Use proper JWT verification with the configured secret:

```go
func replayActorFromRequest(r *http.Request, jwtSecret string) string {
    if v := strings.TrimSpace(r.Header.Get("X-Operator")); v != "" {
        return v
    }
    if v := strings.TrimSpace(r.Header.Get("X-User")); v != "" {
        return v
    }

    authz := strings.TrimSpace(r.Header.Get("Authorization"))
    if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
        token := strings.TrimSpace(authz[len("Bearer "):])
        if token != "" && jwtSecret != "" {
            claims := jwt.MapClaims{}
            parsedToken, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
                if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                    return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
                }
                return []byte(jwtSecret), nil
            })
            if err == nil && parsedToken.Valid {
                for _, key := range []string{"email", "sub", "preferred_username", "name"} {
                    if value := claimString(claims[key]); value != "" {
                        return value
                    }
                }
            }
        }
    }

    ip := clientIP(r.RemoteAddr)
    if ip == "" {
        return "unknown"
    }
    return "ip:" + ip
}
```

**Update callers:**
- `ReplayFailedEvent` (line 225): `actor := replayActorFromRequest(r, h.cfg.AdminJWTSecret)`
- `ReplayFailedEventsBatch` (line 316): `actor := replayActorFromRequest(r, h.cfg.AdminJWTSecret)`

---

### 10. Grafana default credentials — `docker-compose.yml:56-57`

**Problem:** `admin:admin` are well-known defaults.

**Fix:** Use env var placeholders:

```yaml
grafana:
  image: grafana/grafana:11.2.0
  container_name: teampulse-grafana
  environment:
    GF_SECURITY_ADMIN_USER: ${GRAFANA_ADMIN_USER:-admin}
    GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_ADMIN_PASSWORD:-}
  # ...
```

Add to `.env.example`:
```
# Grafana (must be changed from defaults)
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=  # Set a strong password
```

---

### 11. File permissions — all file stores

**Problem:** Files created with `0o644` (world-readable).

**Fix:** Change to `0o600` in all file stores:

```
failstore/file_store.go:104:       0o644 → 0o600
securityaudit/file_store.go:106:   0o644 → 0o600
securityaudit/file_store.go:245:   0o644 → 0o600
replayaudit/file_store.go:131:     0o644 → 0o600
```

---

### 12. Missing fsync — all file stores

**Problem:** No `f.Sync()` before close — data loss on crash.

**Fix:** Add `f.Sync()` before close in all `Save` methods:

```go
// failstore/file_store.go — after f.Write, before defer close
if _, err := f.Write(append(line, '\n')); err != nil {
    return FailedEvent{}, fmt.Errorf("write failed event: %w", err)
}
if err := f.Sync(); err != nil {
    return FailedEvent{}, fmt.Errorf("sync failed event store: %w", err)
}

// securityaudit/file_store.go — same pattern
// replayaudit/file_store.go — same pattern
```

---

### 13. os.Exit skips cleanup — `cmd/server/main.go:375`

**Problem:** `os.Exit(1)` in goroutine bypasses all deferred cleanup.

**Fix:** Use a channel to signal the main goroutine to shut down:

```go
// Add at top of main():
serverErr := make(chan error, 1)

// Replace the goroutine:
go func() {
    logger.Info("ingestion-gateway listening", "port", cfg.Port)
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Error("server failed", "error", err)
        serverErr <- err
    }
}()

// Replace the signal handling:
stop := make(chan os.Signal, 1)
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

select {
case <-stop:
    signal.Stop(stop)
case err := <-serverErr:
    logger.Error("server error triggered shutdown", "error", err)
}

// ... rest of graceful shutdown unchanged
```

---

## MEDIUM SEVERITY FIXES

### 14. Discarded errors in batch replay — `handlers/admin.go:323`

**Problem:** Error from `executeReplay` discarded with `_`.

**Fix:** Log the error:

```go
for _, eventID := range eventIDs {
    result, err := h.executeReplay(r.Context(), replayExecutionInput{
        EventID:         eventID,
        DryRun:          req.DryRun,
        HeaderOverrides: req.HeaderOverrides,
        Actor:           actor,
        RequestID:       requestID,
    })
    if err != nil {
        h.logger.Warn("batch replay event failed",
            "event_id", eventID,
            "error", err,
        )
    }
    results = append(results, result)
    // ... rest unchanged
}
```

---

### 15. Missing metrics on Teams write failure — `handlers/webhooks.go:195-197`

**Problem:** Early return without calling `h.observe()`.

**Fix:** Always call observe before return:

```go
if token := r.URL.Query().Get("validationToken"); token != "" {
    w.Header().Set("Content-Type", "text/plain")
    w.WriteHeader(http.StatusOK)
    _, writeErr := w.Write([]byte(token))
    h.observe(r.Context(), "teams", http.StatusOK)
    if writeErr != nil {
        return
    }
    return
}
```

---

### 16. Silent config fallback — `config/config.go:272-282`

**Problem:** Invalid env vars silently fall back to default.

**Fix:** Log a warning when fallback occurs:

```go
func intOrDefault(key string, fallback int, logger *slog.Logger) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    n, err := strconv.Atoi(v)
    if err != nil {
        if logger != nil {
            logger.Warn("invalid integer config, using default", "key", key, "value", v, "default", fallback)
        }
        return fallback
    }
    if n < 0 {
        if logger != nil {
            logger.Warn("negative integer config, using default", "key", key, "value", n, "default", fallback)
        }
        return fallback
    }
    return n
}
```

**Note:** This requires threading a logger through `LoadFromEnv()`. Alternatively, log to `os.Stderr` directly:

```go
func intOrDefault(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    n, err := strconv.Atoi(v)
    if err != nil {
        fmt.Fprintf(os.Stderr, "WARNING: invalid integer config %s=%q, using default %d\n", key, v, fallback)
        return fallback
    }
    if n < 0 {
        fmt.Fprintf(os.Stderr, "WARNING: negative integer config %s=%d, using default %d\n", key, n, fallback)
        return fallback
    }
    return n
}
```

---

### 17. UTF-8 boundary split — `handlers/admin.go:568-577`

**Problem:** `clean[:limit]` can split multi-byte UTF-8 characters.

**Fix:** Truncate at rune boundary:

```go
func bodyPreview(body []byte, limit int) string {
    if len(body) == 0 || limit <= 0 {
        return ""
    }
    clean := strings.TrimSpace(string(body))
    if len(clean) <= limit {
        return clean
    }
    // Truncate at rune boundary
    truncated := clean[:limit]
    for len(truncated) > 0 && !utf8.ValidString(truncated) {
        truncated = truncated[:len(truncated)-1]
    }
    return truncated + "...truncated"
}
```

Add `"unicode/utf8"` to imports.

---

### 18. Background context in retry scheduler — `retry/scheduler.go:101`

**Problem:** Uses `context.Background()` — retries can't be cancelled during shutdown.

**Fix:** Already fixed in #3 above — now uses `context.WithTimeout(context.Background(), 30*time.Second)`. For full lifecycle awareness, pass a context to `Start()`:

```go
func (s *Scheduler) Start(ctx context.Context) {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.running {
        return
    }
    s.running = true
    s.ticker = time.NewTicker(s.interval)
    s.ctx = ctx  // store for use in processRetries
    go s.run()
}

// Add ctx field to struct:
type Scheduler struct {
    // ... existing fields
    ctx context.Context
}
```

Then in `processRetries`, use `s.ctx` instead of creating a new context.

---

### 19. Redundant check — `cmd/server/main.go:238`

**Problem:** `strings.TrimSpace(in.Source) == ""` already computed at line 222.

**Fix:** Simplify:

```go
recordSecurityEvent := func(req *http.Request, in securityaudit.SaveInput) {
    reqCtx := req.Context()
    source := strings.TrimSpace(in.Source)
    if source == "" {
        source = securitySourceFromPath(req.URL.Path)
    }
    // ... other field processing ...

    // Replace lines 238-240:
    in.Source = source  // Always set, no redundant check
```

---

### 20. Log injection — `httpx/middleware.go:215`

**Problem:** `r.URL.Path` logged directly — attacker can inject fake log entries.

**Fix:** Sanitize the path before logging:

```go
func AccessLog(logger *slog.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
            next.ServeHTTP(rec, r)
            logger.Info("http_request",
                "request_id", RequestIDFromContext(r.Context()),
                "method", r.Method,
                "path", sanitizeLogValue(r.URL.Path),
                "status", rec.status,
                "duration_ms", time.Since(start).Milliseconds(),
                "bytes", rec.size,
                "remote_addr", sanitizeLogValue(r.RemoteAddr),
            )
        })
    }
}

func sanitizeLogValue(s string) string {
    return strings.Map(func(r rune) rune {
        if r < 0x20 || r == 0x7F {
            return -1
        }
        return r
    }, s)
}
```

---

### 21. Path validation in replay CLI — `cmd/replay/main.go:94`

**Problem:** `-file` flag reads arbitrary paths with no validation.

**Fix:** Validate path is within working directory or an allowed data directory:

```go
func loadEventFromFile(path, sourceOverride string, headerOverrides map[string]string) (replay.Event, error) {
    absPath, err := filepath.Abs(path)
    if err != nil {
        return replay.Event{}, apperr.New("cmd.replay.loadFile", apperr.CodeReplayReadFailed, "invalid replay file path", err)
    }
    cwd, err := os.Getwd()
    if err != nil {
        return replay.Event{}, apperr.New("cmd.replay.loadFile", apperr.CodeReplayReadFailed, "cannot determine working directory", err)
    }
    if !strings.HasPrefix(absPath, cwd) {
        return replay.Event{}, apperr.New("cmd.replay.loadFile", apperr.CodeReplayReadFailed,
            fmt.Errorf("file path must be within current directory: %s", path))
    }
    raw, err := os.ReadFile(path)
    // ... rest unchanged
}
```

Add `"path/filepath"` to imports.

---

## LOW SEVERITY FIXES

### 22. Inconsistent error codes — `handlers/webhooks.go:72` vs `handlers/admin.go:55`

**Problem:** Same failure type uses different error codes.

**Fix:** Use `apperr.CodeFailedEventStore` consistently in `admin.go`:

```go
// admin.go line 55: change CodeReplayConfigInvalid → CodeFailedEventStore
logger.Error("admin failed-event explorer disabled due to invalid configuration",
    "path", cfg.FailedStorePath,
    "error", err,
    "error_code", apperr.CodeFailedEventStore,  // was CodeReplayConfigInvalid
)
```

---

### 23. Truncated SHA256 — `handlers/webhooks.go:489`

**Problem:** Only first 16 bytes of SHA256 used for fallback event ID.

**Fix:** Use full hash:

```go
func fallbackEventID(source string, body []byte) string {
    sum := sha256.Sum256(body)
    return fmt.Sprintf("%s_%s", source, hex.EncodeToString(sum[:]))
}
```

---

### 24. Negative value handling — `config/config.go:278`

**Problem:** `n < 0` silently falls back — `DEDUP_TTL_SEC=0` gets default 300 instead of disabled.

**Fix:** Allow zero (meaning disabled) and only reject truly invalid values:

```go
func intOrDefault(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    n, err := strconv.Atoi(v)
    if err != nil {
        fmt.Fprintf(os.Stderr, "WARNING: invalid integer config %s=%q, using default %d\n", key, v, fallback)
        return fallback
    }
    return n  // Allow zero and negative; let Validate() catch invalid ranges
}
```

---

### 25. Error wrapping in pubsub_publisher — `queue/pubsub_publisher.go:41`

**Problem:** Close error not in error chain (`%v` not `%w`).

**Fix:** Create a custom error type or wrap both:

```go
if _, err := client.TopicAdminClient.GetTopic(ctx, &pubsubpb.GetTopicRequest{Topic: topicName}); err != nil {
    if closeErr := client.Close(); closeErr != nil {
        return nil, fmt.Errorf("check topic existence: %w; client close error: %w", err, closeErr)
    }
    // ... rest unchanged
}
```

---

### 26. Sensitive data in bodyPreview — `handlers/admin.go:173, 568-577`

**Problem:** Up to 220 chars of raw webhook body exposed (may contain secrets).

**Fix:** Redact common secret patterns:

```go
func bodyPreview(body []byte, limit int) string {
    if len(body) == 0 || limit <= 0 {
        return ""
    }
    clean := strings.TrimSpace(string(body))
    // Redact common secret patterns
    clean = redactSecrets(clean)
    if len(clean) <= limit {
        return clean
    }
    truncated := clean[:limit]
    for len(truncated) > 0 && !utf8.ValidString(truncated) {
        truncated = truncated[:len(truncated)-1]
    }
    return truncated + "...truncated"
}

func redactSecrets(s string) string {
    patterns := []string{
        `"token"`, `"secret"`, `"password"`, `"key"`, `"authorization"`,
    }
    for _, pattern := range patterns {
        idx := strings.Index(strings.ToLower(s), pattern)
        if idx != -1 {
            // Find the value after the pattern
            colonIdx := strings.Index(s[idx:], ":")
            if colonIdx != -1 {
                start := idx + colonIdx + 1
                // Find end of value (comma, quote, or brace)
                end := start
                for end < len(s) && s[end] != ',' && s[end] != '}' && s[end] != '\n' {
                    end++
                }
                if end > start {
                    s = s[:start] + "[REDACTED]" + s[end:]
                }
            }
        }
    }
    return s
}
```

---

## SECURITY DEFAULTS (Fail-Secure Posture)

### 27. Enable security controls by default — `config/config.go`

**Fix:** Change defaults:

```go
// Line 71: REQUIRE_SECRETS default
RequireSecrets: boolOrDefault("REQUIRE_SECRETS", true),  // was false in .env but true here already

// Line 75: ADMIN_AUTH_ENABLED default
AdminAuthEnabled: boolOrDefault("ADMIN_AUTH_ENABLED", true),  // was false

// Line 100: SOURCE_RATE_LIMIT_ENABLED default
SourceRateLimitEnabled: boolOrDefault("SOURCE_RATE_LIMIT_ENABLED", true),  // was false

// Line 103: SCHEMA_VALIDATION_ENABLED default
SchemaValidationEnabled: boolOrDefault("SCHEMA_VALIDATION_ENABLED", true),  // was false
```

Update `.env` and `.env.example` accordingly:

```
REQUIRE_SECRETS=true
ADMIN_AUTH_ENABLED=true
```

---

## SUMMARY OF ALL CHANGES

| # | File | Change | Severity |
|---|------|--------|----------|
| 1 | `handlers/health.go` | Nil check in `checkQueue` | CRITICAL |
| 2 | `retry/scheduler.go` | Safe type assertions `val.(int)` → `val.(int); ok` | CRITICAL |
| 3 | `retry/scheduler.go` | `math/rand` → `crypto/rand` for jitter | HIGH |
| 4 | `httpx/middleware.go` | Panic on nil `cfg.Limiter` or return cleanup func | CRITICAL |
| 5 | `httpx/middleware.go` | `sync.Once` for `IPRateLimiter.Stop()` | CRITICAL |
| 6 | `httpx/middleware.go` | Minimum 1s window validation | CRITICAL |
| 7 | `dedup/memory.go` | `sync.Once` for `Memory.Stop()` | CRITICAL |
| 8 | `.env`, `.env.example` | Remove `change-me` secret | HIGH |
| 9 | `observability/telemetry.go` | Remove `WithInsecure()`, fix sampling rate | HIGH |
| 10 | `handlers/admin.go` | Proper JWT verification for actor resolution | HIGH |
| 11 | `docker-compose.yml` | Env var placeholders for Grafana creds | HIGH |
| 12 | `failstore/file_store.go` | `0o644` → `0o600`, add `f.Sync()` | HIGH |
| 13 | `securityaudit/file_store.go` | `0o644` → `0o600`, add `f.Sync()` | HIGH |
| 14 | `replayaudit/file_store.go` | `0o644` → `0o600`, add `f.Sync()` | HIGH |
| 15 | `cmd/server/main.go` | `os.Exit(1)` → channel-based graceful shutdown | HIGH |
| 16 | `handlers/admin.go` | Log discarded errors in batch replay | MEDIUM |
| 17 | `handlers/webhooks.go` | Always call `observe()` in Teams handler | MEDIUM |
| 18 | `config/config.go` | Warn on invalid config fallback | MEDIUM |
| 19 | `handlers/admin.go` | UTF-8 safe truncation in `bodyPreview` | MEDIUM |
| 20 | `retry/scheduler.go` | Timeout context instead of `context.Background()` | MEDIUM |
| 21 | `cmd/server/main.go` | Remove redundant `in.Source` check | MEDIUM |
| 22 | `httpx/middleware.go` | Sanitize log values to prevent injection | MEDIUM |
| 23 | `cmd/replay/main.go` | Validate file path within working directory | MEDIUM |
| 24 | `handlers/admin.go` | Consistent error code `CodeFailedEventStore` | LOW |
| 25 | `handlers/webhooks.go` | Full SHA256 hash for fallback event ID | LOW |
| 26 | `config/config.go` | Allow zero in `intOrDefault`, warn on parse error | LOW |
| 27 | `queue/pubsub_publisher.go` | Wrap both errors with `%w` | LOW |
| 28 | `handlers/admin.go` | Redact secrets in `bodyPreview` | LOW |
| 29 | `config/config.go` | Fail-secure defaults (secrets, auth, rate limits) | HIGH |
