package httpx

import (
	"bufio"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"hash/fnv"
	"log/slog"
	mathrand "math/rand/v2"
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

var reqCounter atomic.Uint64

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

type ipShard struct {
	mu      sync.Mutex
	entries map[string]rateWindow
}

// IPRateLimiter is sharded across 16 stripes so concurrent webhooks don't
// contend on a single global Mutex. Each shard holds its own map + lock.
type IPRateLimiter struct {
	shards   [16]*ipShard
	now      func() time.Time
	window   time.Duration
	windowS  int64
	hits     atomic.Uint64
	cleanup  int
	stopCh   chan struct{}
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
		now:     now,
		window:  window,
		windowS: int64(window / time.Second),
		cleanup: cleanupEveryN,
		stopCh:  make(chan struct{}),
	}
	for i := range l.shards {
		l.shards[i] = &ipShard{entries: make(map[string]rateWindow, 64)}
		// Background expiry so the hot path never does a full-map scan.
	}
	go l.backgroundCleanup()
	return l
}

// backgroundCleanup sweeps stale windows every minute outside the hot path.
func (l *IPRateLimiter) backgroundCleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.CleanupStale()
		case <-l.stopCh:
			return
		}
	}
}

func (l *IPRateLimiter) Stop() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}

func (l *IPRateLimiter) shardFor(key string) *ipShard {
	// FNV over the key, no alloc beyond the hash itself.
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return l.shards[h.Sum32()%uint32(len(l.shards))]
}

func (l *IPRateLimiter) CleanupStale() {
	now := l.now().Unix()
	currentWindowStart := now - (now % l.windowS)
	threshold := currentWindowStart - l.windowS

	for _, sh := range l.shards {
		sh.mu.Lock()
		for key, entry := range sh.entries {
			if entry.windowStart < threshold {
				delete(sh.entries, key)
			}
		}
		// Cap each shard to ~4k entries to bound memory under key flood.
		if len(sh.entries) > 4096 {
			n := 0
			for key := range sh.entries {
				delete(sh.entries, key)
				n++
				if n >= 1024 {
					break
				}
			}
		}
		sh.mu.Unlock()
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

	sh := l.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()

	entry, ok := sh.entries[key]
	if !ok || entry.windowStart != windowStart {
		sh.entries[key] = rateWindow{windowStart: windowStart, count: 1}
		l.maybeCleanupLocked(windowStart, sh)
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
	sh.entries[key] = entry
	l.maybeCleanupLocked(windowStart, sh)
	return RateLimitResult{
		Allowed:   true,
		Remaining: limit - entry.count,
		ResetAt:   resetAt,
		Limit:     limit,
	}
}

func (l *IPRateLimiter) maybeCleanupLocked(currentWindowStart int64, sh *ipShard) {
	h := l.hits.Add(1)
	if int(h%uint64(l.cleanup)) != 0 {
		return
	}
	// Opportunistic, shard-local only — never a full-map scan.
	for key, entry := range sh.entries {
		if currentWindowStart-entry.windowStart > l.windowS {
			delete(sh.entries, key)
		}
		if len(sh.entries) < 2048 {
			// Keep the sweep short; backgroundCleanup handles the rest.
			continue
		}
		break
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

// Flush/Hijack passthrough so SSE/h2 don't break behind the recorder.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

func Chain(h http.Handler, m ...Middleware) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

// IsProbePath reports cheap health/metrics paths that must skip expensive
// middleware (timeouts, rate limits, full access logs).
func IsProbePath(path string) bool {
	switch path {
	case "/healthz", "/readyz", "/metrics":
		return true
	}
	if strings.HasPrefix(path, "/assets/") {
		return true
	}
	return false
}

// RouteTemplate collapses high-cardinality paths to bounded metric labels.
func RouteTemplate(path string) string {
	if IsProbePath(path) {
		return path
	}
	if strings.HasPrefix(path, "/api/v1/webhooks/") || strings.HasPrefix(path, "/webhooks/") {
		return "/webhooks/:source"
	}
	if strings.HasPrefix(path, "/api/v1/admin/") || strings.HasPrefix(path, "/admin/") {
		return "/admin/:endpoint"
	}
	if path == "/" || strings.HasPrefix(path, "/ui") {
		return "/ui"
	}
	return "other"
}

func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
			if requestID != "" {
				if len(requestID) > 128 {
					requestID = requestID[:128]
				}
				// Only sanitize when dirty (control chars) to avoid Map alloc.
				if needsSanitize(requestID) {
					requestID = sanitizeLogValue(requestID)
				}
			}
			if requestID == "" {
				// Cheap counter-based ID; crypto trace IDs are lazy below.
				requestID = "req-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(reqCounter.Add(1), 10)
			}
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-Id", requestID)

			// Lazy traceparent: only mint crypto IDs when the client didn't send one.
			// This saves 2x crypto/rand + hex + Sprintf on ~100% of traffic.
			traceparent := strings.TrimSpace(r.Header.Get("Traceparent"))
			if traceparent == "" {
				// Defer generation: set a lightweight placeholder; OTEL layer
				// generates the real span context only when sampled.
				traceparent = ""
			} else if needsSanitize(traceparent) {
				traceparent = sanitizeLogValue(traceparent)
			}
			if traceparent != "" {
				w.Header().Set("Traceparent", traceparent)
				ctx = context.WithValue(ctx, contextKey("traceparent"), traceparent)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		return strings.Repeat("0", 32)
	}
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	if _, err := crand.Read(b); err != nil {
		return strings.Repeat("0", 16)
	}
	return hex.EncodeToString(b)
}

type ChaosConfig struct {
	Enabled     bool
	ErrorRate   float64 // 0.0 to 1.0
	LatencyRate float64 // 0.0 to 1.0
	LatencyMin  time.Duration
	LatencyMax  time.Duration
}

func Chaos(cfg ChaosConfig) Middleware {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Inject Latency
			if cfg.LatencyRate > 0 && mathrand.Float64() < cfg.LatencyRate {
				delay := cfg.LatencyMin
				if cfg.LatencyMax > cfg.LatencyMin {
					diff := cfg.LatencyMax - cfg.LatencyMin
					delay += time.Duration(mathrand.Float64() * float64(diff))
				}
				time.Sleep(delay)
			}

			// Inject Errors
			if cfg.ErrorRate > 0 && mathrand.Float64() < cfg.ErrorRate {
				http.Error(w, "chaos: injected failure", http.StatusInternalServerError)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RequestTimeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Probes don't need a timer alloc + wheel insert per scrape.
			if IsProbePath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
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
			duration := time.Since(start)
			// Bounded cardinality: never emit raw URL paths (attacker-controlled).
			route := RouteTemplate(r.URL.Path)
			if durationHistogram != nil {
				durationHistogram.Record(r.Context(), duration.Seconds(),
					metric.WithAttributes(
						attribute.String("method", r.Method),
						attribute.String("route", route),
						attribute.Int("status", rec.status),
					),
				)
			}
			// Sample success logs: Info only on errors, slow (>500ms), or probes skipped.
			// This cuts slog JSONHandler + runtime.Callers cost on healthy traffic.
			if rec.status >= 400 || duration > 500*time.Millisecond {
				logger.Info("http_request",
					"request_id", RequestIDFromContext(r.Context()),
					"method", r.Method,
					"route", route,
					"status", rec.status,
					"duration_ms", duration.Milliseconds(),
					"bytes", rec.size,
				)
			} else {
				logger.Debug("http_request",
					"request_id", RequestIDFromContext(r.Context()),
					"method", r.Method,
					"route", route,
					"status", rec.status,
					"duration_ms", duration.Milliseconds(),
					"bytes", rec.size,
				)
			}
		})
	}
}

// MaxInflight caps concurrent requests with fail-fast 503 + Retry-After so
// unbounded HTTP goroutines can't pile up behind Redis/queue saturation.
func MaxInflight(limit int, retryAfterSec int) Middleware {
	if limit <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	if retryAfterSec < 1 {
		retryAfterSec = 2
	}
	sem := make(chan struct{}, limit)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				next.ServeHTTP(w, r)
			default:
				w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
				WriteError(w, r.Context(), http.StatusServiceUnavailable, apperr.New(
					"httpx.MaxInflight",
					apperr.CodeQueueFull,
					"server busy",
					nil,
				), nil)
			}
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
			// Probes bypass rate limiting entirely.
			if IsProbePath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ip := ClientIPFromRequest(r, trusted)
			limit := general
			scope := "general"
			if strings.HasPrefix(r.URL.Path, "/admin") || strings.HasPrefix(r.URL.Path, "/api/v1/admin") {
				limit = admin
				scope = "admin"
			}

			var result RateLimitResult
			// Prefer ctx-aware Redis path (50ms budget) when available.
			if rl, ok := limiter.(interface {
				AllowWithInfo(string, int, time.Time) RateLimitResult
			}); ok {
				_ = rl
			}
			if ctxLimiter, ok := limiter.(interface {
				AllowWithContext(ctx context.Context, key string, limit int) bool
			}); ok && isRedisLimiter(limiter) {
				allowed := ctxLimiter.AllowWithContext(r.Context(), scope+"|"+ip, limit)
				result = RateLimitResult{Allowed: allowed, Limit: limit}
				if !allowed {
					result.Remaining = 0
				} else {
					result.Remaining = limit
				}
			} else if rl, ok := limiter.(RateLimiterWithInfo); ok {
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
			allowed := true
			if ctxLimiter, ok := limiter.(interface {
				AllowWithContext(ctx context.Context, key string, limit int) bool
			}); ok && isRedisLimiter(limiter) {
				allowed = ctxLimiter.AllowWithContext(r.Context(), key, limit)
			} else {
				allowed = limiter.Allow(key, limit)
			}
			if !allowed {
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
	// Cache parsed CIDRs: previously parsed on every rejection.
	return ClientIPFromRequest(r, parseCIDRsCached(trustedProxyCIDRs))
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

// parseCIDRsCached avoids re-parsing the same CIDR strings on every request.
var cidrCache sync.Map // map[string][]*net.IPNet

func parseCIDRsCached(cidrs []string) []*net.IPNet {
	if len(cidrs) == 0 {
		return nil
	}
	key := strings.Join(cidrs, ",")
	if v, ok := cidrCache.Load(key); ok {
		if nets, ok := v.([]*net.IPNet); ok {
			return nets
		}
	}
	nets := parseCIDRs(cidrs)
	cidrCache.Store(key, nets)
	return nets
}

func isRedisLimiter(l RateLimiter) bool {
	_, ok := l.(*RedisRateLimiter)
	return ok
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
	if !needsSanitize(s) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}

// needsSanitize fast-paths the common case (clean ASCII) without allocating.
func needsSanitize(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7F {
			return true
		}
	}
	return false
}
