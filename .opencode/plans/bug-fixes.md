# Bug Fixes & Code Cleanup Plan

## Summary
This plan addresses 6 issues found in the TeamPulseBridge ingestion-gateway service, ranging from critical memory leaks to dead code removal.

---

## Fix 1: Memory Leak in Dedup Store (HIGH PRIORITY)

**File:** `services/ingestion-gateway/internal/dedup/memory.go`

**Problem:** The `Seen()` method only performs opportunistic cleanup when `len(seen) > 10,000`. Under low-to-moderate traffic, expired entries accumulate indefinitely, causing unbounded memory growth.

**Solution:** Add a background goroutine that periodically cleans up expired entries based on TTL.

**Changes:**
```go
// Add stop channel to struct
type Memory struct {
    enabled bool
    ttl     time.Duration
    mu      sync.Mutex
    seen    map[string]time.Time
    stop    chan struct{}  // NEW
}

// Update constructor to start cleanup goroutine
func NewMemory(enabled bool, ttl time.Duration) *Memory {
    if ttl <= 0 {
        ttl = 5 * time.Minute
    }
    m := &Memory{
        enabled: enabled,
        ttl:     ttl,
        seen:    make(map[string]time.Time),
        stop:    make(chan struct{}),
    }
    go m.periodicCleanup()  // NEW
    return m
}

// NEW: Periodic cleanup method
func (m *Memory) periodicCleanup() {
    ticker := time.NewTicker(m.ttl / 2)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            m.cleanupExpired()
        case <-m.stop:
            return
        }
    }
}

// NEW: Cleanup expired entries
func (m *Memory) cleanupExpired() {
    now := time.Now().UTC()
    m.mu.Lock()
    defer m.mu.Unlock()
    for k, ts := range m.seen {
        if !ts.After(now) {
            delete(m.seen, k)
        }
    }
}

// NEW: Stop method for graceful shutdown
func (m *Memory) Stop() {
    close(m.stop)
}

// Simplify Seen() - remove opportunistic cleanup
func (m *Memory) Seen(key string) bool {
    if !m.enabled || key == "" {
        return false
    }
    now := time.Now().UTC()
    expiry := now.Add(m.ttl)
    m.mu.Lock()
    defer m.mu.Unlock()
    if existing, ok := m.seen[key]; ok && existing.After(now) {
        return true
    }
    m.seen[key] = expiry
    return false  // Removed the 10,000 entry cleanup block
}
```

**Integration needed in:** `cmd/server/main.go`
- Call `deduper.Stop()` during shutdown sequence

---

## Fix 2: Double Handler Build Waste (HIGH PRIORITY)

**File:** `services/ingestion-gateway/cmd/server/main.go` (lines 290-299)

**Problem:** `handlerBuilder.Build()` is called twice. The first build is completely discarded, wasting resources and creating duplicate middleware chains.

**Current code:**
```go
publicMux := newPublicMux(webhookMux, nil)  // Line 290
handlerBuilder := NewHandlerBuilder(logger, &cfg, securityRejectFn)
handler := handlerBuilder.Build(publicMux)  // First build - DISCARDED

uiSmokeProxy := handlers.NewUISmokeTestProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    handler.ServeHTTP(w, r)
}), cfg.TrustedProxyCIDRs)
publicMux = newPublicMux(webhookMux, uiSmokeProxy)  // Line 298
handler = handlerBuilder.Build(publicMux)  // Second build - USED
```

**Solution:** Build handler only once with the final `publicMux`:
```go
handlerBuilder := NewHandlerBuilder(logger, &cfg, securityRejectFn)

uiSmokeProxy := handlers.NewUISmokeTestProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Will use handler after it's built
}), cfg.TrustedProxyCIDRs)

publicMux := newPublicMux(webhookMux, uiSmokeProxy)
handler := handlerBuilder.Build(publicMux)

// Update the proxy to use the built handler
uiSmokeProxy.SetHandler(handler)
```

**Alternative (simpler):** If `UISmokeTestProxy` doesn't need the handler reference during construction:
```go
handlerBuilder := NewHandlerBuilder(logger, &cfg, securityRejectFn)

uiSmokeProxy := handlers.NewUISmokeTestProxy(webhookMux, cfg.TrustedProxyCIDRs)
publicMux := newPublicMux(webhookMux, uiSmokeProxy)
handler := handlerBuilder.Build(publicMux)
```

---

## Fix 3: Context Error Returns Without Response (MEDIUM PRIORITY)

**File:** `services/ingestion-gateway/internal/handlers/webhooks.go`

**Problem:** When `r.Context().Err()` is not nil (client disconnected), the handler returns without writing any HTTP response. This leaves the client hanging until timeout.

**Locations:**
- Line 107-109: `HandleSlack`
- Line 143-145: `HandleGitHub`
- Line 162-164: `HandleGitLab`
- Line 181-183: `HandleTeams`

**Current code:**
```go
if err := r.Context().Err(); err != nil {
    return  // No response written!
}
```

**Solution:** Write a proper HTTP response before returning:
```go
if err := r.Context().Err(); err != nil {
    h.observe(r.Context(), "slack", 499)  // Client Closed Request
    return
}
```

**Note:** Status 499 is Nginx convention for "Client Closed Request". Alternatively, we could skip the `observe` call since the client is already gone, or use 408 (Request Timeout).

---

## Fix 4: Remove Dead Code - RateLimitOptions (MEDIUM PRIORITY)

**File:** `services/ingestion-gateway/internal/httpx/middleware.go` (lines 26-28)

**Problem:** `RateLimitOptions` struct is defined but never used anywhere in the codebase.

**Current code:**
```go
type RateLimitOptions struct {
    CleanupFunc func()
}
```

**Solution:** Delete this struct entirely. It's dead code that adds confusion.

---

## Fix 5: Signal Channel Leak (LOW PRIORITY)

**File:** `services/ingestion-gateway/cmd/server/main.go` (line 318-319)

**Problem:** `signal.Notify(stop, ...)` registers the channel to receive signals but `signal.Stop(stop)` is never called to unregister it.

**Current code:**
```go
stop := make(chan os.Signal, 1)
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
<-stop
// No signal.Stop(stop) call
```

**Solution:** Add cleanup after receiving the signal:
```go
stop := make(chan os.Signal, 1)
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
<-stop
signal.Stop(stop)  // NEW: Unregister the signal channel
```

---

## Fix 6: Event ID Collision Risk (LOW PRIORITY)

**File:** `services/ingestion-gateway/internal/handlers/webhooks.go` (line 465-468)

**Problem:** `fallbackEventID()` uses only the first 8 bytes of the SHA256 hash, creating only 16 hex characters. This increases collision probability significantly compared to using the full hash.

**Current code:**
```go
func fallbackEventID(source string, body []byte) string {
    sum := sha256.Sum256(body)
    return fmt.Sprintf("%s_%s", source, hex.EncodeToString(sum[:8]))  // Only 8 bytes
}
```

**Solution:** Use more bytes from the hash to reduce collision probability:
```go
func fallbackEventID(source string, body []byte) string {
    sum := sha256.Sum256(body)
    return fmt.Sprintf("%s_%s", source, hex.EncodeToString(sum[:16]))  // 16 bytes = 32 hex chars
}
```

**Tradeoff:** Slightly longer event IDs (32 hex chars vs 16), but dramatically lower collision probability (2^128 vs 2^64 space).

---

## Implementation Order

1. **Fix 1** - Dedup memory leak (critical, affects production stability)
2. **Fix 2** - Double handler build (wasteful, easy fix)
3. **Fix 3** - Context error handling (improves reliability)
4. **Fix 4** - Dead code removal (code hygiene)
5. **Fix 5** - Signal cleanup (minor leak)
6. **Fix 6** - Event ID collision (safety improvement)

## Testing Recommendations

After applying fixes:
1. Run existing test suite: `go test ./...`
2. Run linter: `golangci-lint run`
3. Verify dedup cleanup with a test that creates entries and waits for TTL
4. Verify handler builds only once with a simple integration test
5. Test signal handling with `kill -TERM` in a test environment

## Risk Assessment

- **Fix 1:** Low risk - adds standard cleanup pattern, backward compatible
- **Fix 2:** Low risk - removes redundant code, behavior unchanged
- **Fix 3:** Low risk - only affects error path, improves behavior
- **Fix 4:** Zero risk - removes unused code
- **Fix 5:** Zero risk - adds standard cleanup
- **Fix 6:** Low risk - changes event ID format slightly, but more robust
