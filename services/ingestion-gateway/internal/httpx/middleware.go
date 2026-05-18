package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
)

type contextKey string

const requestIDKey contextKey = "request_id"

var reqCounter uint64

type Middleware func(http.Handler) http.Handler

type RateLimiter interface {
	Allow(key string, limit int) bool
}

type RateLimitResult struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time
	Limit     int
}

type RateLimiterWithInfo interface {
	AllowWithInfo(key string, limit int, now time.Time) RateLimitResult
}

type RateLimitConfig struct {
	Enabled           bool
	General           int
	Admin             int
	TrustedProxyCIDRs []string
	OnReject          func(r *http.Request, reason string, status int)
	Now               func() time.Time
	Window            time.Duration
	CleanupN          int
	Limiter           RateLimiter
}

type rateWindow struct {
	windowStart int64
	count       int
}

type IPRateLimiter struct {
	mu       sync.Mutex
	entries  map[string]rateWindow
	now      func() time.Time
	window   time.Duration
	windowS  int64
	hits     uint64
	cleanup  int
	stop     chan struct{}
	stopOnce sync.Once
}

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

func (l *IPRateLimiter) Stop() {
	l.stopOnce.Do(func() {
		close(l.stop)
	})
}

func (l *IPRateLimiter) periodicCleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.CleanupStale()
		case <-l.stop:
			return
		}
	}
}

func (l *IPRateLimiter) CleanupStale() {
	now := l.now().Unix()
	currentWindowStart := now - (now % l.windowS)
	threshold := currentWindowStart - l.windowS

	l.mu.Lock()
	defer l.mu.Unlock()

	for key, entry := range l.entries {
		if entry.windowStart < threshold {
			delete(l.entries, key)
		}
	}
}

func (l *IPRateLimiter) Allow(key string, limit int) bool {
	result := l.AllowWithInfo(key, limit, l.now())
	return result.Allowed
}

func (l *IPRateLimiter) AllowWithInfo(key string, limit int, now time.Time) RateLimitResult {
	if limit <= 0 {
		return RateLimitResult{Allowed: false, Limit: limit}
	}
	unix := now.Unix()
	windowStart := unix - (unix % l.windowS)
	resetAt := time.Unix(windowStart+l.windowS, 0)

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok || entry.windowStart != windowStart {
		l.entries[key] = rateWindow{windowStart: windowStart, count: 1}
		l.maybeCleanupLocked(windowStart)
		return RateLimitResult{
			Allowed:   true,
			Remaining: limit - 1,
			ResetAt:   resetAt,
			Limit:     limit,
		}
	}
	if entry.count >= limit {
		return RateLimitResult{
			Allowed:   false,
			Remaining: 0,
			ResetAt:   resetAt,
			Limit:     limit,
		}
	}
	entry.count++
	l.entries[key] = entry
	l.maybeCleanupLocked(windowStart)
	return RateLimitResult{
		Allowed:   true,
		Remaining: limit - entry.count,
		ResetAt:   resetAt,
		Limit:     limit,
	}
}

func (l *IPRateLimiter) maybeCleanupLocked(currentWindowStart int64) {
	h := atomic.AddUint64(&l.hits, 1)
	if h%uint64(l.cleanup) != 0 {
		return
	}
	for key, entry := range l.entries {
		if currentWindowStart-entry.windowStart > l.windowS {
			delete(l.entries, key)
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

func Chain(h http.Handler, m ...Middleware) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
			if requestID != "" {
				requestID = sanitizeLogValue(requestID)
				if len(requestID) > 128 {
					requestID = requestID[:128]
				}
			}
			if requestID == "" {
				requestID = fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&reqCounter, 1))
			}
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-Id", requestID)

			traceparent := strings.TrimSpace(r.Header.Get("Traceparent"))
			if traceparent == "" {
				traceID := generateTraceID()
				spanID := generateSpanID()
				traceparent = fmt.Sprintf("00-%s-%s-01", traceID, spanID)
			}
			w.Header().Set("Traceparent", traceparent)
			ctx = context.WithValue(ctx, contextKey("traceparent"), traceparent)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", 32)
	}
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", 16)
	}
	return hex.EncodeToString(b)
}

type ChaosConfig struct {
	Enabled      bool
	ErrorRate    float64 // 0.0 to 1.0
	LatencyRate  float64 // 0.0 to 1.0
	LatencyMin   time.Duration
	LatencyMax   time.Duration
	InternalCode int
}

func Chaos(cfg ChaosConfig) Middleware {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Randomness for chaos
			randVal := func() float64 {
				b := make([]byte, 8)
				if _, err := rand.Read(b); err != nil {
					return 0
				}
				var val uint64
				for i := 0; i < 8; i++ {
					val = (val << 8) | uint64(b[i])
				}
				return float64(val) / float64(^uint64(0))
			}

			// Inject Latency
			if cfg.LatencyRate > 0 && randVal() < cfg.LatencyRate {
				delay := cfg.LatencyMin
				if cfg.LatencyMax > cfg.LatencyMin {
					diff := cfg.LatencyMax - cfg.LatencyMin
					delay += time.Duration(randVal() * float64(diff))
				}
				time.Sleep(delay)
			}

			// Inject Errors
			if cfg.ErrorRate > 0 && randVal() < cfg.ErrorRate {
				code := cfg.InternalCode
				if code == 0 {
					code = http.StatusInternalServerError
				}
				http.Error(w, "chaos: injected failure", code)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RequestTimeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Recoverer(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered",
						"panic", recovered,
						"request_id", RequestIDFromContext(r.Context()),
						"stack", string(debug.Stack()),
					)
					WriteError(w, r.Context(), http.StatusInternalServerError, apperr.New(
						"httpx.Recoverer",
						apperr.CodeInternalServerError,
						"internal server error",
						nil,
					), nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func AccessLog(logger *slog.Logger, durationHistogram metric.Float64Histogram) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			duration := time.Since(start).Seconds()
			if durationHistogram != nil {
				durationHistogram.Record(r.Context(), duration,
					metric.WithAttributes(
						attribute.String("method", r.Method),
						attribute.String("path", sanitizeLogValue(r.URL.Path)),
						attribute.Int("status", rec.status),
					),
				)
			}
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

func RateLimit(cfg RateLimitConfig) Middleware {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}
	if cfg.Limiter == nil {
		panic("httpx.RateLimit: cfg.Limiter is required to prevent goroutine leaks")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	limiter := cfg.Limiter
	trusted := parseCIDRs(cfg.TrustedProxyCIDRs)
	general := cfg.General
	admin := cfg.Admin

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIPFromRequest(r, trusted)
			limit := general
			scope := "general"
			if strings.HasPrefix(r.URL.Path, "/admin") {
				limit = admin
				scope = "admin"
			}

			var result RateLimitResult
			if rl, ok := limiter.(RateLimiterWithInfo); ok {
				result = rl.AllowWithInfo(scope+"|"+ip, limit, cfg.Now())
			} else {
				allowed := limiter.Allow(scope+"|"+ip, limit)
				result = RateLimitResult{Allowed: allowed, Limit: limit}
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			if !result.ResetAt.IsZero() {
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
			}

			if !result.Allowed {
				if cfg.OnReject != nil {
					cfg.OnReject(r, "rate_limit_exceeded", http.StatusTooManyRequests)
				}
				w.Header().Set("Retry-After", "60")
				WriteError(w, r.Context(), http.StatusTooManyRequests, apperr.New(
					"httpx.RateLimit",
					apperr.CodeRateLimitExceeded,
					"rate limit exceeded",
					nil,
				), nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type SourceRateLimitConfig struct {
	Enabled           bool
	Sources           map[string]int
	Default           int
	TrustedProxyCIDRs []string
	Limiter           RateLimiter
	OnReject          func(r *http.Request, source string, status int)
}

func SourceRateLimit(cfg SourceRateLimitConfig) Middleware {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}
	if cfg.Limiter == nil {
		panic("httpx.SourceRateLimit: cfg.Limiter is required to prevent goroutine leaks")
	}
	limiter := cfg.Limiter
	trusted := parseCIDRs(cfg.TrustedProxyCIDRs)
	if cfg.Default <= 0 {
		cfg.Default = 100
	}

	sourceFromPath := func(path string) string {
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

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			source := sourceFromPath(r.URL.Path)
			if source == "" {
				next.ServeHTTP(w, r)
				return
			}

			limit, exists := cfg.Sources[source]
			if !exists {
				limit = cfg.Default
			}

			ip := ClientIPFromRequest(r, trusted)
			key := "source|" + source + "|" + ip
			if !limiter.Allow(key, limit) {
				if cfg.OnReject != nil {
					cfg.OnReject(r, source, http.StatusTooManyRequests)
				}
				w.Header().Set("Retry-After", "60")
				WriteError(w, r.Context(), http.StatusTooManyRequests, apperr.New(
					"httpx.SourceRateLimit",
					apperr.CodeRateLimitExceeded,
					"rate limit exceeded for source: "+source,
					nil,
				), nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ClientIPFromRequest(r *http.Request, trustedProxyNets []*net.IPNet) string {
	if len(trustedProxyNets) == 0 {
		return clientIPNoProxyTrust(r)
	}
	remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		remoteHost = strings.TrimSpace(r.RemoteAddr)
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP != nil && ipInNets(remoteIP, trustedProxyNets) {
		xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
		if xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(xr) != nil {
			return xr
		}
	}
	return clientIPNoProxyTrust(r)
}

func ClientIP(r *http.Request, trustedProxyCIDRs []string) string {
	return ClientIPFromRequest(r, parseCIDRs(trustedProxyCIDRs))
}

func clientIPNoProxyTrust(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func parseCIDRs(cidrs []string) []*net.IPNet {
	if len(cidrs) == 0 {
		return nil
	}
	result := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			continue
		}
		result = append(result, network)
	}
	return result
}

func ipInNets(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func sanitizeLogValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}
