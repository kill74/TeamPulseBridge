package httpx

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	HeaderAPIVersion     = "X-API-Version"
	HeaderDeprecation    = "Deprecation"
	HeaderSunset         = "Sunset"
	HeaderLink           = "Link"
	CurrentAPIVersion    = "v1"
	LegacyRoutePrefix    = "/webhooks/"
	VersionedRoutePrefix = "/api/v1/webhooks/"
	LegacyAdminPrefix    = "/admin/"
	VersionedAdminPrefix = "/api/v1/admin/"
)

var DefaultSunsetDate = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

type APIVersionConfig struct {
	Enabled           bool
	SunsetDate        time.Time
	Version           string
	OnVersionMismatch func(w http.ResponseWriter, r *http.Request, requested string)
}

func APIVersionMiddleware(cfg APIVersionConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	sunset := cfg.SunsetDate
	if sunset.IsZero() {
		sunset = DefaultSunsetDate
	}
	version := cfg.Version
	if version == "" {
		version = CurrentAPIVersion
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			// Define mappings for legacy to versioned routes
			mappings := []struct {
				legacy    string
				versioned string
				apiPath   string
			}{
				{LegacyRoutePrefix, VersionedRoutePrefix, "webhooks"},
				{LegacyAdminPrefix, VersionedAdminPrefix, "admin"},
			}

			isVersioned := false
			for _, m := range mappings {
				if strings.HasPrefix(path, m.legacy) && !strings.HasPrefix(path, m.versioned) {
					w.Header().Set(HeaderDeprecation, "true")
					w.Header().Set(HeaderSunset, sunset.Format(time.RFC1123))
					w.Header().Set(HeaderLink, fmt.Sprintf(`</api/%s/%s%s>; rel="successor-version"`, version, m.apiPath, strings.TrimPrefix(path, m.legacy)))
					w.Header().Set(HeaderAPIVersion, "legacy")
					break
				} else if strings.HasPrefix(path, m.versioned) {
					w.Header().Set(HeaderAPIVersion, version)
					isVersioned = true
					break
				}
			}

			if !isVersioned && !strings.HasPrefix(path, "/api/") {
				// Not an API route or already handled legacy route
			}

			requestedVersion := r.Header.Get(HeaderAPIVersion)
			if requestedVersion != "" && requestedVersion != "legacy" && requestedVersion != version {
				if cfg.OnVersionMismatch != nil {
					cfg.OnVersionMismatch(w, r, requestedVersion)
					return
				}
				http.Error(w, fmt.Sprintf("unsupported API version: %s (current: %s)", requestedVersion, version), http.StatusNotAcceptable)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RegisterVersionedRoutes(mux *http.ServeMux, handlers map[string]http.HandlerFunc, prefix string) {
	for source, handler := range handlers {
		path := prefix + source
		mux.HandleFunc(path, handler)
	}
}
