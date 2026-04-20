package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewPublicMuxRoutesSmokeEndpointSeparately(t *testing.T) {
	appHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		_, _ = w.Write([]byte(r.URL.Path))
	})
	smokeHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("smoke"))
	})

	mux := newPublicMux(appHandler, smokeHandler)

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRR := httptest.NewRecorder()
	mux.ServeHTTP(healthRR, healthReq)

	if healthRR.Code != http.StatusNoContent {
		t.Fatalf("expected /healthz to reach app handler, got %d", healthRR.Code)
	}
	if got := strings.TrimSpace(healthRR.Body.String()); got != "/healthz" {
		t.Fatalf("expected app handler body /healthz, got %q", got)
	}

	smokeReq := httptest.NewRequest(http.MethodPost, "/ui/smoke-test", strings.NewReader(`{}`))
	smokeRR := httptest.NewRecorder()
	mux.ServeHTTP(smokeRR, smokeReq)

	if smokeRR.Code != http.StatusAccepted {
		t.Fatalf("expected /ui/smoke-test to reach smoke handler, got %d", smokeRR.Code)
	}
	if got := strings.TrimSpace(smokeRR.Body.String()); got != "smoke" {
		t.Fatalf("expected smoke handler body smoke, got %q", got)
	}
}
