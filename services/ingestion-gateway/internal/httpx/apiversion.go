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

			if strings.HasPrefix(path, LegacyRoutePrefix) && !strings.HasPrefix(path, VersionedRoutePrefix) {
				w.Header().Set(HeaderDeprecation, "true")
				w.Header().Set(HeaderSunset, sunset.Format(time.RFC1123))
				w.Header().Set(HeaderLink, fmt.Sprintf(`</api/%s%s>; rel="successor-version"`, version, strings.TrimPrefix(path, "/webhooks")))
				w.Header().Set(HeaderAPIVersion, "legacy")
			} else if strings.HasPrefix(path, VersionedRoutePrefix) {
				w.Header().Set(HeaderAPIVersion, version)
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
