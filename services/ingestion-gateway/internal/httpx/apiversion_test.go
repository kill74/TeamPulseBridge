package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIVersionMiddleware_LegacyRoute(t *testing.T) {
	cfg := APIVersionConfig{
		Enabled:    true,
		Version:    "v1",
		SunsetDate: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	middleware := APIVersionMiddleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if dep := rr.Header().Get(HeaderDeprecation); dep != "true" {
		t.Errorf("expected Deprecation header 'true', got %q", dep)
	}

	if sunset := rr.Header().Get(HeaderSunset); sunset == "" {
		t.Error("expected Sunset header to be set")
	}

	if link := rr.Header().Get(HeaderLink); link == "" {
		t.Error("expected Link header to be set")
	}

	if ver := rr.Header().Get(HeaderAPIVersion); ver != "legacy" {
		t.Errorf("expected X-API-Version 'legacy', got %q", ver)
	}
}

func TestAPIVersionMiddleware_VersionedRoute(t *testing.T) {
	cfg := APIVersionConfig{
		Enabled:    true,
		Version:    "v1",
		SunsetDate: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	middleware := APIVersionMiddleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if dep := rr.Header().Get(HeaderDeprecation); dep != "" {
		t.Errorf("expected no Deprecation header for versioned route, got %q", dep)
	}

	if ver := rr.Header().Get(HeaderAPIVersion); ver != "v1" {
		t.Errorf("expected X-API-Version 'v1', got %q", ver)
	}
}

func TestAPIVersionMiddleware_VersionMismatch(t *testing.T) {
	cfg := APIVersionConfig{
		Enabled:    true,
		Version:    "v1",
		SunsetDate: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		OnVersionMismatch: func(w http.ResponseWriter, r *http.Request, requested string) {
			http.Error(w, "unsupported version", http.StatusNotAcceptable)
		},
	}
	middleware := APIVersionMiddleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", nil)
	req.Header.Set(HeaderAPIVersion, "v2")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Errorf("expected status 406, got %d", rr.Code)
	}
}

func TestAPIVersionMiddleware_VersionMatch(t *testing.T) {
	cfg := APIVersionConfig{
		Enabled:    true,
		Version:    "v1",
		SunsetDate: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	middleware := APIVersionMiddleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", nil)
	req.Header.Set(HeaderAPIVersion, "v1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestAPIVersionMiddleware_Disabled(t *testing.T) {
	cfg := APIVersionConfig{
		Enabled: false,
	}
	middleware := APIVersionMiddleware(cfg)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	if dep := rr.Header().Get(HeaderDeprecation); dep != "" {
		t.Errorf("expected no Deprecation header when disabled, got %q", dep)
	}
}

func TestRegisterVersionedRoutesAllowsHandlerToOwnMethodValidation(t *testing.T) {
	mux := http.NewServeMux()
	RegisterVersionedRoutes(mux, map[string]http.HandlerFunc{
		"teams": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("expected GET to reach handler, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		},
	}, "/webhooks/")

	req := httptest.NewRequest(http.MethodGet, "/webhooks/teams?validationToken=test", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected GET route to reach handler, got %d", rr.Code)
	}
}

func TestRegisterVersionedRoutes(t *testing.T) {
	mux := http.NewServeMux()
	handlers := map[string]http.HandlerFunc{
		"github": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
		"slack": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	}

	RegisterVersionedRoutes(mux, handlers, "/api/v1/webhooks/")

	tests := []struct {
		path   string
		expect int
	}{
		{"/api/v1/webhooks/github", http.StatusOK},
		{"/api/v1/webhooks/slack", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != tt.expect {
				t.Errorf("path %s: expected status %d, got %d", tt.path, tt.expect, rr.Code)
			}
		})
	}
}
