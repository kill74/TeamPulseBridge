package httpx

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRateLimitGeneralExceeded(t *testing.T) {
	now := fixedNow(time.Unix(1700000000, 0))
	h := RateLimit(RateLimitConfig{Enabled: true, General: 2, Admin: 1, Now: now, Window: time.Minute})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}

	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr3.Code)
	}
}

func TestRateLimitAdminUsesStricterLimit(t *testing.T) {
	now := fixedNow(time.Unix(1700000100, 0))
	h := RateLimit(RateLimitConfig{Enabled: true, General: 5, Admin: 1, Now: now, Window: time.Minute})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/configz", nil)
	req.RemoteAddr = "10.0.0.2:12345"

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr2.Code)
	}
}

func TestRateLimitResetsOnNextWindow(t *testing.T) {
	clk := &clock{t: time.Unix(1700000200, 0)}
	h := RateLimit(RateLimitConfig{Enabled: true, General: 1, Admin: 1, Now: clk.Now, Window: time.Minute})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	req.RemoteAddr = "10.0.0.3:1111"

	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr2.Code)
	}

	clk.Add(61 * time.Second)
	rr3 := httptest.NewRecorder()
	h.ServeHTTP(rr3, req)
	if rr3.Code != http.StatusOK {
		t.Fatalf("expected 200 after window reset, got %d", rr3.Code)
	}
}

func TestRateLimitDoesNotTrustXFFWithoutTrustedProxy(t *testing.T) {
	now := fixedNow(time.Unix(1700000300, 0))
	h := RateLimit(RateLimitConfig{Enabled: true, General: 1, Admin: 1, Now: now, Window: time.Minute})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqA := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqA.RemoteAddr = "198.51.100.10:1234"
	reqA.Header.Set("X-Forwarded-For", "10.1.1.1")
	rrA := httptest.NewRecorder()
	h.ServeHTTP(rrA, reqA)
	if rrA.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", rrA.Code)
	}

	reqB := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqB.RemoteAddr = "198.51.100.10:5678"
	reqB.Header.Set("X-Forwarded-For", "10.1.1.2")
	rrB := httptest.NewRecorder()
	h.ServeHTTP(rrB, reqB)
	if rrB.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 because limiter should key by remote addr, got %d", rrB.Code)
	}
}

func TestRateLimitTrustsXFFFromTrustedProxy(t *testing.T) {
	now := fixedNow(time.Unix(1700000400, 0))
	h := RateLimit(RateLimitConfig{Enabled: true, General: 1, Admin: 1, Now: now, Window: time.Minute, TrustedProxyCIDRs: []string{"198.51.100.0/24"}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqA := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqA.RemoteAddr = "198.51.100.10:1234"
	reqA.Header.Set("X-Forwarded-For", "10.1.1.1")
	rrA := httptest.NewRecorder()
	h.ServeHTTP(rrA, reqA)
	if rrA.Code != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", rrA.Code)
	}

	reqB := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqB.RemoteAddr = "198.51.100.10:5678"
	reqB.Header.Set("X-Forwarded-For", "10.1.1.2")
	rrB := httptest.NewRecorder()
	h.ServeHTTP(rrB, reqB)
	if rrB.Code != http.StatusOK {
		t.Fatalf("expected 200 because trusted proxy should key by forwarded client ip, got %d", rrB.Code)
	}
}

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
