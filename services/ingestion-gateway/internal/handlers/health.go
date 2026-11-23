package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"teampulsebridge/services/ingestion-gateway/internal/dedup"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

type RedisHealthChecker interface {
	Ping(ctx context.Context) error
}

type redisPingWrapper struct {
	pingFunc func(ctx context.Context) error
}

func (w *redisPingWrapper) Ping(ctx context.Context) error {
	return w.pingFunc(ctx)
}

func NewRedisPingWrapper(pingFunc func(ctx context.Context) error) RedisHealthChecker {
	return &redisPingWrapper{pingFunc: pingFunc}
}

type HealthChecker struct {
	publisher   queue.Publisher
	failStore   failstore.Store
	deduper     dedup.Store
	redisClient RedisHealthChecker
	startTime   time.Time
	// Coalesce concurrent /healthz scrapes (kube + prom can stampede).
	sf        singleflight.Group
	mu        sync.Mutex
	cached    healthResponse
	cachedAt  time.Time
	cachedTTL time.Duration
}

func NewHealthChecker(publisher queue.Publisher, failStore failstore.Store, deduper dedup.Store) *HealthChecker {
	return &HealthChecker{
		publisher: publisher,
		failStore: failStore,
		deduper:   deduper,
		startTime: time.Now().UTC(),
		cachedTTL: 2 * time.Second,
	}
}

func NewHealthCheckerWithRedis(publisher queue.Publisher, failStore failstore.Store, deduper dedup.Store, redisClient RedisHealthChecker) *HealthChecker {
	return &HealthChecker{
		publisher:   publisher,
		failStore:   failStore,
		deduper:     deduper,
		redisClient: redisClient,
		startTime:   time.Now().UTC(),
		cachedTTL:   2 * time.Second,
	}
}

type healthResponse struct {
	Status     string                     `json:"status"`
	UptimeSec  float64                    `json:"uptime_sec"`
	Components map[string]componentHealth `json:"components"`
}

type componentHealth struct {
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latency_ms,omitempty"`
	Entries   int     `json:"entries,omitempty"`
	Error     string  `json:"error,omitempty"`
}

func (h *HealthChecker) Healthz(w http.ResponseWriter, r *http.Request) {
	// 2s memoize + singleflight: concurrent kube + prom scrapes share one probe.
	h.mu.Lock()
	if h.cachedTTL > 0 && !h.cachedAt.IsZero() && time.Since(h.cachedAt) < h.cachedTTL && len(h.cached.Components) > 0 {
		cached := h.cached
		cached.UptimeSec = time.Since(h.startTime).Seconds()
		h.mu.Unlock()
		writeHealthJSON(w, cached)
		return
	}
	h.mu.Unlock()

	v, _, _ := h.sf.Do("healthz", func() (any, error) {
		return h.computeHealth(r.Context()), nil
	})
	resp, _ := v.(healthResponse)
	resp.UptimeSec = time.Since(h.startTime).Seconds()
	h.mu.Lock()
	h.cached = resp
	h.cachedAt = time.Now()
	h.mu.Unlock()
	writeHealthJSON(w, resp)
}

func writeHealthJSON(w http.ResponseWriter, resp healthResponse) {
	w.Header().Set("Content-Type", "application/json")
	statusCode := http.StatusOK
	if resp.Status == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *HealthChecker) computeHealth(ctx context.Context) healthResponse {
	type namedResult struct {
		name   string
		health componentHealth
	}
	results := make(chan namedResult, 4)

	// Fan-out: previously sequential 1s+1s+1s = up to 3s blocked handler.
	go func() { results <- namedResult{"queue", h.checkQueue(ctx)} }()
	if h.failStore != nil {
		go func() { results <- namedResult{"fail_store", h.checkFailStore(ctx)} }()
	}
	if h.redisClient != nil {
		go func() { results <- namedResult{"redis", h.checkRedis(ctx)} }()
	}

	components := make(map[string]componentHealth)
	overallStatus := "healthy"

	// Dedup is config-only, no I/O.
	if h.deduper != nil {
		components["dedup"] = componentHealth{Status: "ok"}
	} else {
		components["dedup"] = componentHealth{Status: "disabled"}
	}
	if h.failStore == nil {
		components["fail_store"] = componentHealth{Status: "disabled"}
	}
	if h.redisClient == nil {
		components["redis"] = componentHealth{Status: "disabled"}
	}

	expected := 1
	if h.failStore != nil {
		expected++
	}
	if h.redisClient != nil {
		expected++
	}
	timeout := time.After(1500 * time.Millisecond)
	for i := 0; i < expected; i++ {
		select {
		case res := <-results:
			components[res.name] = res.health
			if res.health.Status != "ok" && res.health.Status != "disabled" {
				overallStatus = "degraded"
			}
		case <-timeout:
			overallStatus = "degraded"
			// Fill missing with timeout status.
			if _, ok := components["queue"]; !ok {
				components["queue"] = componentHealth{Status: "error", Error: "health check timeout"}
			}
			i = expected // break
		case <-ctx.Done():
			overallStatus = "degraded"
			i = expected
		}
	}

	return healthResponse{
		Status:     overallStatus,
		UptimeSec:  time.Since(h.startTime).Seconds(),
		Components: components,
	}
}

func (h *HealthChecker) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.publisher == nil {
		writeReadyzError(w, "publisher not configured")
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := h.publisher.HealthCheck(checkCtx); err != nil {
		writeReadyzError(w, "queue health check failed: "+err.Error())
		return
	}

	if h.redisClient != nil {
		if err := h.redisClient.Ping(checkCtx); err != nil {
			writeReadyzError(w, "redis health check failed: "+err.Error())
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	}); err != nil {
		return
	}
}

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
	latency := time.Since(start).Seconds() * 1000

	if err != nil {
		return componentHealth{
			Status:    "error",
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}

	return componentHealth{
		Status:    "ok",
		LatencyMs: latency,
	}
}

func (h *HealthChecker) checkFailStore(ctx context.Context) componentHealth {
	checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	_, err := h.failStore.ListRecent(checkCtx, 1)
	if err != nil {
		return componentHealth{
			Status: "error",
			Error:  err.Error(),
		}
	}

	return componentHealth{Status: "ok"}
}

func (h *HealthChecker) checkRedis(ctx context.Context) componentHealth {
	start := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	if h.redisClient == nil {
		return componentHealth{Status: "disabled"}
	}

	err := h.redisClient.Ping(checkCtx)
	latency := time.Since(start).Seconds() * 1000

	if err != nil {
		return componentHealth{
			Status:    "error",
			LatencyMs: latency,
			Error:     err.Error(),
		}
	}

	return componentHealth{
		Status:    "ok",
		LatencyMs: latency,
	}
}

func writeReadyzError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "not_ready",
		"error":  message,
	}); err != nil {
		return
	}
}
