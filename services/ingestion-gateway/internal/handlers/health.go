package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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
	publisher     queue.Publisher
	failStore     failstore.Store
	deduper       dedup.Store
	redisClient   RedisHealthChecker
	startTime     time.Time
}

func NewHealthChecker(publisher queue.Publisher, failStore failstore.Store, deduper dedup.Store) *HealthChecker {
	return &HealthChecker{
		publisher: publisher,
		failStore: failStore,
		deduper:   deduper,
		startTime: time.Now().UTC(),
	}
}

func NewHealthCheckerWithRedis(publisher queue.Publisher, failStore failstore.Store, deduper dedup.Store, redisClient RedisHealthChecker) *HealthChecker {
	return &HealthChecker{
		publisher:   publisher,
		failStore:   failStore,
		deduper:     deduper,
		redisClient: redisClient,
		startTime:   time.Now().UTC(),
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
	ctx := r.Context()
	components := make(map[string]componentHealth)
	overallStatus := "healthy"

	queueStatus := h.checkQueue(ctx)
	components["queue"] = queueStatus
	if queueStatus.Status != "ok" {
		overallStatus = "degraded"
	}

	if h.failStore != nil {
		storeStatus := h.checkFailStore(ctx)
		components["fail_store"] = storeStatus
		if storeStatus.Status != "ok" {
			overallStatus = "degraded"
		}
	} else {
		components["fail_store"] = componentHealth{Status: "disabled"}
	}

	if h.deduper != nil {
		components["dedup"] = componentHealth{Status: "ok"}
	} else {
		components["dedup"] = componentHealth{Status: "disabled"}
	}

	if h.redisClient != nil {
		redisStatus := h.checkRedis(ctx)
		components["redis"] = redisStatus
		if redisStatus.Status != "ok" {
			overallStatus = "degraded"
		}
	} else {
		components["redis"] = componentHealth{Status: "disabled"}
	}

	resp := healthResponse{
		Status:     overallStatus,
		UptimeSec:  time.Since(h.startTime).Seconds(),
		Components: components,
	}

	w.Header().Set("Content-Type", "application/json")
	statusCode := http.StatusOK
	if overallStatus == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
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
