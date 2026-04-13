package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestRequireAdminJWT_AllowsValidToken(t *testing.T) {
	cfg := JWTConfig{Enabled: true, Issuer: "teampulse", Audience: "ops", Secret: "secret"}
	h := RequireAdminJWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/configz", nil)
	req.Header.Set("Authorization", "Bearer "+mintToken(t, cfg))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAdminJWT_RejectsMissingToken(t *testing.T) {
	cfg := JWTConfig{Enabled: true, Issuer: "teampulse", Audience: "ops", Secret: "secret"}
	h := RequireAdminJWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/configz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestRequireAdminJWT_SkipsWebhookPaths(t *testing.T) {
	cfg := JWTConfig{Enabled: true, Issuer: "teampulse", Audience: "ops", Secret: "secret"}
	h := RequireAdminJWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
}

func TestRequireAdminCIDRAllowlist_AllowsAdminRequestFromAllowedCIDR(t *testing.T) {
	h := RequireAdminCIDRAllowlist(AdminCIDRConfig{Enabled: true, CIDRs: []string{"10.0.0.0/8"}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/configz", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAdminCIDRAllowlist_RejectsAdminRequestOutsideCIDR(t *testing.T) {
	h := RequireAdminCIDRAllowlist(AdminCIDRConfig{Enabled: true, CIDRs: []string{"10.0.0.0/8"}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/configz", nil)
	req.RemoteAddr = "192.168.1.10:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdminCIDRAllowlist_SkipsWebhookPaths(t *testing.T) {
	h := RequireAdminCIDRAllowlist(AdminCIDRConfig{Enabled: true, CIDRs: []string{"10.0.0.0/8"}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	req.RemoteAddr = "192.168.1.10:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
}

func TestRequireAdminCIDRAllowlist_DoesNotTrustXFFWithoutTrustedProxy(t *testing.T) {
	h := RequireAdminCIDRAllowlist(AdminCIDRConfig{Enabled: true, CIDRs: []string{"10.0.0.0/8"}})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/configz", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "10.1.2.3")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdminCIDRAllowlist_TrustsXFFFromTrustedProxy(t *testing.T) {
	h := RequireAdminCIDRAllowlist(AdminCIDRConfig{
		Enabled:           true,
		CIDRs:             []string{"10.0.0.0/8"},
		TrustedProxyCIDRs: []string{"203.0.113.0/24"},
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/configz", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("X-Forwarded-For", "10.1.2.3")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func mintToken(t *testing.T, cfg JWTConfig) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": cfg.Issuer,
		"aud": cfg.Audience,
		"exp": time.Now().Add(5 * time.Minute).Unix(),
		"iat": time.Now().Unix(),
	})
	s, err := token.SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return s
}
