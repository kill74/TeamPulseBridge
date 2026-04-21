package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
)

type contextKey string

const requestIDKey contextKey = "request_id"

var reqCounter uint64

type Middleware func(http.Handler) http.Handler

type RateLimitConfig struct {
	Enabled           bool
	General           int
	Admin             int
	TrustedProxyCIDRs []string
	OnReject          func(r *http.Request, reason string, status int)
	Now               func() time.Time
	Window            time.Duration
	CleanupN          int
}

type rateWindow struct {
	windowStart int64
	count       int
}

type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateWindow
	now     func() time.Time
	window  time.Duration
	windowS int64
	hits    uint64
	cleanup int
	stop    chan struct{}
}

func newIPRateLimiter(now func() time.Time, window time.Duration, cleanupEveryN int) *ipRateLimiter {
	if now == nil {
		now = time.Now
	}
	if window <= 0 {
		window = time.Minute
	}
	if cleanupEveryN <= 0 {
		cleanupEveryN = 1024
	}
	l := &ipRateLimiter{
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

func (l *ipRateLimiter) Stop() {
	close(l.stop)
}

func (l *ipRateLimiter) periodicCleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.cleanupStale()
		case <-l.stop:
			return
		}
	}
}

func (l *ipRateLimiter) cleanupStale() {
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

func (l *ipRateLimiter) allow(key string, limit int) bool {
	if limit <= 0 {
		return false
	}
	now := l.now().Unix()
	windowStart := now - (now % l.windowS)

	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok || entry.windowStart != windowStart {
		l.entries[key] = rateWindow{windowStart: windowStart, count: 1}
		l.maybeCleanupLocked(windowStart)
		return true
	}
	if entry.count >= limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	l.maybeCleanupLocked(windowStart)
	return true
}

func (l *ipRateLimiter) maybeCleanupLocked(currentWindowStart int64) {
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
			if requestID == "" {
				requestID = fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&reqCounter, 1))
			}
			ctx := context.WithValue(r.Context(), requestIDKey, requestID)
			w.Header().Set("X-Request-Id", requestID)
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

func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Info("http_request",
				"request_id", RequestIDFromContext(r.Context()),
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"bytes", rec.size,
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

func RateLimit(cfg RateLimitConfig) Middleware {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}
	limiter := newIPRateLimiter(cfg.Now, cfg.Window, cfg.CleanupN)
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
			if !limiter.allow(scope+"|"+ip, limit) {
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
