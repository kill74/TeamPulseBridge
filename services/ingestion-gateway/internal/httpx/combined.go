package httpx

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
)

// CombinedGate collapses general rate-limit + source rate-limit + dedup reservation
// into a single Redis Lua invocation (1 RTT instead of 3).
//
// Usage (opt-in via RATE_LIMIT_COMBINED=true in cmd/server/main.go):
//
//	gate := NewCombinedGate(limiter, dedupPrefix, dedupTTL)
//	mux.Use(RateLimitAndDedupCombined(gate, ...))
//
// Fail-open on any Redis error to preserve availability. Duplicates short-circuit
// with 202 accepted (idempotent replay) without reaching the handler.
type CombinedGate struct {
	Limiter     *RedisRateLimiter
	DedupPrefix string
	DedupTTL    time.Duration
}

func NewCombinedGate(limiter *RedisRateLimiter, dedupPrefix string, dedupTTL time.Duration) *CombinedGate {
	if dedupTTL <= 0 {
		dedupTTL = 5 * time.Minute
	}
	return &CombinedGate{Limiter: limiter, DedupPrefix: strings.TrimSpace(dedupPrefix), DedupTTL: dedupTTL}
}

type CombinedGateConfig struct {
	Enabled           bool
	General           int
	Admin             int
	Sources           map[string]int
	SourceDefault     int
	TrustedProxyCIDRs []string
	DedupEnabled      bool
	OnReject          func(r *http.Request, reason string, status int)
	OnDuplicate       func(r *http.Request, source, eventID string)
	// EventIDFromRequest extracts a stable dedup key without reading the body.
	// Return "" to skip dedup for this request (e.g. body-derived IDs).
	EventIDFromRequest func(r *http.Request, source string) string
}

// RateLimitAndDedupCombined replaces RateLimit+SourceRateLimit+handler-Seen with 1 RTT.
// Non-webhook paths only pay the general check; webhook paths pay general+source+dedup.
func RateLimitAndDedupCombined(gate *CombinedGate, cfg CombinedGateConfig) Middleware {
	if !cfg.Enabled || gate == nil || gate.Limiter == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	trusted := parseCIDRsCached(cfg.TrustedProxyCIDRs)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsProbePath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ip := ClientIPFromRequest(r, trusted)
			isAdmin := strings.HasPrefix(r.URL.Path, "/admin") || strings.HasPrefix(r.URL.Path, "/api/v1/admin")
			source := sourceFromWebhookPath(r.URL.Path)

			generalLimit := cfg.General
			if isAdmin {
				generalLimit = cfg.Admin
			}
			scope := "general"
			if isAdmin {
				scope = "admin"
			}
			generalKey := scope + "|" + ip

			sourceKey := ""
			sourceLimit := 0
			if source != "" {
				sourceLimit = cfg.SourceDefault
				if v, ok := cfg.Sources[source]; ok {
					sourceLimit = v
				}
				if sourceLimit <= 0 {
					sourceLimit = cfg.SourceDefault
				}
				sourceKey = "source|" + source + "|" + ip
			}

			dedupKey := ""
			var eventID string
			if cfg.DedupEnabled && source != "" && cfg.EventIDFromRequest != nil {
				if eid := strings.TrimSpace(cfg.EventIDFromRequest(r, source)); eid != "" {
					eventID = eid
					if gate.DedupPrefix != "" {
						dedupKey = gate.DedupPrefix + ":" + source + ":" + eid
					} else {
						dedupKey = "webhook_dedup:" + source + ":" + eid
					}
				}
			}

			res := gate.Limiter.CombinedCheckWithContext(r.Context(), generalKey, sourceKey, dedupKey, generalLimit, sourceLimit, gate.DedupTTL)

			if !res.GeneralAllowed {
				if cfg.OnReject != nil {
					cfg.OnReject(r, "rate_limit_exceeded", http.StatusTooManyRequests)
				}
				w.Header().Set("Retry-After", "60")
				w.Header().Set("X-RateLimit-Limit", strconv.Itoa(generalLimit))
				w.Header().Set("X-RateLimit-Remaining", "0")
				WriteError(w, r.Context(), http.StatusTooManyRequests, apperr.New(
					"httpx.CombinedGate", apperr.CodeRateLimitExceeded, "rate limit exceeded", nil,
				), nil)
				return
			}
			if sourceKey != "" && !res.SourceAllowed {
				if cfg.OnReject != nil {
					cfg.OnReject(r, "source_rate_limit_exceeded:"+source, http.StatusTooManyRequests)
				}
				w.Header().Set("Retry-After", "60")
				WriteError(w, r.Context(), http.StatusTooManyRequests, apperr.New(
					"httpx.CombinedGate", apperr.CodeRateLimitExceeded, "rate limit exceeded for source: "+source, nil,
				), nil)
				return
			}
			if dedupKey != "" && res.IsDuplicate {
				if cfg.OnDuplicate != nil {
					cfg.OnDuplicate(r, source, eventID)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"status":"accepted","duplicate":true}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func sourceFromWebhookPath(path string) string {
	for _, prefix := range []string{"/webhooks/", "/api/v1/webhooks/"} {
		if strings.HasPrefix(path, prefix) {
			source := strings.TrimPrefix(path, prefix)
			source = strings.Trim(source, "/")
			if idx := strings.Index(source, "/"); idx != -1 {
				source = source[:idx]
			}
			return source
		}
	}
	return ""
}
