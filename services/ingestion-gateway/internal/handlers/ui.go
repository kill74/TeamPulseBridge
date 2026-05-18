package handlers

import (
	"net"
	"net/http"
	"strings"
)

const uiAssetVersion = "20260419.1"

func clientIP(remoteAddr string) string {
	ip := strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(ip); err == nil && host != "" {
		return strings.Trim(host, "[]")
	}
	if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(ip, "["), "]")
	}
	return ip
}

func setUISecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Origin-Agent-Cluster", "?1")
}

func setUICSPHeader(w http.ResponseWriter) {
	csp := "default-src 'self'; " +
		"script-src 'self'; " +
		"style-src 'self' https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com; " +
		"img-src 'self' data:; " +
		"connect-src 'self'; " +
		"base-uri 'none'; " +
		"frame-ancestors 'none'; " +
		"form-action 'self'"
	w.Header().Set("Content-Security-Policy", csp)
}

func setAssetCachingHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("ETag", `"ui-`+uiAssetVersion+`"`)
}

func isNotModified(r *http.Request, w http.ResponseWriter) bool {
	if match := r.Header.Get("If-None-Match"); match == `"ui-`+uiAssetVersion+`"` {
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	return false
}

// ProductUI serves the operator console page.
func ProductUI(w http.ResponseWriter, _ *http.Request) {
	setUISecurityHeaders(w)
	setUICSPHeader(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if err := productUITemplate.Execute(w, struct{ Version string }{Version: uiAssetVersion}); err != nil {
		return
	}
}

// ProductUIStyles serves versioned CSS for the product UI.
func ProductUIStyles(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setAssetCachingHeaders(w)
	if isNotModified(r, w) {
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	if _, err := w.Write([]byte(productUICSS)); err != nil {
		return
	}
}

// ProductUIScript serves versioned JS for the product UI.
func ProductUIScript(w http.ResponseWriter, r *http.Request) {
	setUISecurityHeaders(w)
	setAssetCachingHeaders(w)
	if isNotModified(r, w) {
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	if _, err := w.Write([]byte(productUIJS)); err != nil {
		return
	}
}
