# Retry Scheduler Bug Fixes

## Issues Found

### 1. CRITICAL: Headers Race Condition (Line 118-122)
**Problem:** `headers := event.Headers` creates a reference to the original map. Modifying it mutates the original event's headers, causing race conditions.

**Fix:** Create a copy of the headers map before modification.

### 2. CRITICAL: RetryCount Never Incremented (Line 103-114)
**Problem:** The scheduler checks `event.RetryCount >= s.maxRetries` but never increments RetryCount after retry attempts. Events will retry infinitely.

**Fix:** Track retried event IDs in-memory with their attempt counts using a sync.Map.

### 3. CRITICAL: Double Stop Panic (Line 80)
**Problem:** `close(s.stop)` will panic if `Stop()` is called multiple times.

**Fix:** Use `sync.Once` to ensure the stop channel is only closed once.

### 4. MINOR: Global rand Not Thread-Safe (Line 155)
**Problem:** Using global `rand.Float64()` is not thread-safe in Go < 1.20.

**Fix:** Use a dedicated `rand.Rand` instance with proper seeding.

---

## Implementation Plan

### Step 1: Add retry tracking to Scheduler struct
```go
type Scheduler struct {
    store      failstore.Store
    publisher  queue.Publisher
    logger     *slog.Logger
    maxRetries int
    interval   time.Duration
    ticker     *time.Ticker
    stop       chan struct{}
    stopOnce   sync.Once          // NEW: Prevent double-close panic
    mu         sync.Mutex
    running    bool
    onRetry    func(ctx context.Context, source string, success bool, attempt int)
    rng        *rand.Rand         // NEW: Thread-safe random
    retries    sync.Map           // NEW: Track retry counts in-memory (eventID -> int)
}
```

### Step 2: Update NewScheduler
```go
func NewScheduler(store failstore.Store, publisher queue.Publisher, logger *slog.Logger, opts SchedulerOptions) *Scheduler {
    if opts.MaxRetries <= 0 {
        opts.MaxRetries = 3
    }
    if opts.Interval <= 0 {
        opts.Interval = 10 * time.Second
    }

    return &Scheduler{
        store:      store,
        publisher:  publisher,
        logger:     logger,
        maxRetries: opts.MaxRetries,
        interval:   opts.Interval,
        stop:       make(chan struct{}),
        onRetry:    opts.OnRetry,
        rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
    }
}
```

### Step 3: Fix Stop() with sync.Once
```go
func (s *Scheduler) Stop() {
    s.mu.Lock()
    if !s.running {
        s.mu.Unlock()
        return
    }
    s.running = false
    s.mu.Unlock()
    
    s.stopOnce.Do(func() {
        if s.ticker != nil {
            s.ticker.Stop()
        }
        close(s.stop)
    })
}
```

### Step 4: Fix retryEvent() - Copy headers and track retries
```go
func (s *Scheduler) retryEvent(ctx context.Context, event failstore.FailedEvent) {
    // Track retry count in-memory
    currentRetries := 0
    if val, ok := s.retries.Load(event.EventID); ok {
        currentRetries = val.(int)
    }
    nextRetries := currentRetries + 1
    
    // Check max retries
    if nextRetries > s.maxRetries {
        s.logger.Info("max retries exceeded, skipping",
            "event_id", event.EventID,
            "max_retries", s.maxRetries,
        )
        return
    }
    
    // Update retry count
    s.retries.Store(event.EventID, nextRetries)
    
    // Copy headers to avoid race condition
    headers := make(map[string]string, len(event.Headers)+1)
    for k, v := range event.Headers {
        headers[k] = v
    }
    headers["X-Retry-Count"] = fmt.Sprintf("%d", nextRetries)

    err := s.publisher.Publish(ctx, event.Source, event.Body, headers)

    success := err == nil
    if s.onRetry != nil {
        s.onRetry(ctx, event.Source, success, nextRetries)
    }

    if success {
        s.logger.Info("event retried successfully",
            "event_id", event.EventID,
            "source", event.Source,
            "attempt", nextRetries,
        )
        // Clean up tracking on success
        s.retries.Delete(event.EventID)
    } else {
        s.logger.Warn("event retry failed",
            "event_id", event.EventID,
            "source", event.Source,
            "attempt", nextRetries,
            "error", err,
        )
    }
}
```

### Step 5: Fix calculateBackoff to use instance rand
```go
func (s *Scheduler) calculateBackoff(retryCount int, baseInterval time.Duration) time.Duration {
    if retryCount < 0 {
        retryCount = 0
    }

    exp := math.Pow(2, float64(retryCount))
    backoff := time.Duration(float64(baseInterval) * exp)

    s.mu.Lock()
    jitter := time.Duration(s.rng.Float64() * float64(baseInterval))
    s.mu.Unlock()
    backoff += jitter

    maxBackoff := 5 * time.Minute
    if backoff > maxBackoff {
        backoff = maxBackoff
    }

    return backoff
}
```

### Step 6: Update processRetries to use instance method
```go
func (s *Scheduler) processRetries() {
    ctx := context.Background()

    events, err := s.store.ListRecent(ctx, 50)
    if err != nil {
        s.logger.Error("failed to list events for retry", "error", err)
        return
    }

    for _, event := range events {
        // Check in-memory retry count
        currentRetries := 0
        if val, ok := s.retries.Load(event.EventID); ok {
            currentRetries = val.(int)
        }
        
        if currentRetries >= s.maxRetries {
            continue
        }

        backoff := s.calculateBackoff(currentRetries, s.interval)
        if time.Since(event.FailedAt) < backoff {
            continue
        }

        s.retryEvent(ctx, event)
    }
}
```

---

## Testing Plan

1. **Test headers isolation:** Verify original event headers are not modified
2. **Test max retries:** Verify events stop retrying after maxRetries attempts
3. **Test double stop:** Verify calling Stop() twice doesn't panic
4. **Test thread safety:** Run with `-race` flag to detect data races
5. **Test backoff calculation:** Verify exponential backoff with jitter works correctly

---

## Risk Assessment

- **Low Risk:** All fixes are additive and don't change existing behavior
- **Backward Compatible:** In-memory tracking doesn't affect file store
- **Performance:** sync.Map is optimized for concurrent read-heavy workloads
- **Memory:** sync.Map entries are cleaned up on successful retry
