package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

const (
	uiSmokeMaxBodyBytes         = 256 * 1024
	uiSmokeResponseBodyMaxBytes = 16 * 1024
	uiSmokeMaxRequestsPerMinute = 20
)

type uiSmokeRequest struct {
	Endpoint string            `json:"endpoint"`
	Payload  json.RawMessage   `json:"payload"`
	Headers  map[string]string `json:"headers"`
}

type uiSmokeResponse struct {
	Endpoint string            `json:"endpoint"`
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body"`
}

func isAllowedSmokeEndpoint(endpoint string) bool {
	switch endpoint {
	case "/webhooks/slack", "/webhooks/teams", "/webhooks/github", "/webhooks/gitlab":
		return true
	default:
		return false
	}
}

func sanitizeUISmokeHeaders(headers map[string]string) map[string]string {
	clean := map[string]string{}
	for k, v := range headers {
		key := strings.TrimSpace(k)
		value := strings.TrimSpace(v)
		if key == "" || value == "" {
			continue
		}
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "cookie" || lower == "x-operator" || lower == "x-user" {
			continue
		}
		if lower == "content-type" || strings.HasPrefix(lower, "x-") {
			clean[key] = value
		}
	}
	if _, ok := clean["Content-Type"]; !ok {
		clean["Content-Type"] = "application/json"
	}
	return clean
}

func clientIPFromRequestSmoke(remoteAddr string, header http.Header, trustedProxyNets []*net.IPNet) string {
	if len(trustedProxyNets) == 0 {
		return clientIP(remoteAddr)
	}
	remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		remoteHost = strings.TrimSpace(remoteAddr)
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP != nil && ipInNetsSmoke(remoteIP, trustedProxyNets) {
		xff := strings.TrimSpace(header.Get("X-Forwarded-For"))
		if xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				if ip := strings.TrimSpace(parts[0]); net.ParseIP(ip) != nil {
					return ip
				}
			}
		}
		if xr := strings.TrimSpace(header.Get("X-Real-IP")); net.ParseIP(xr) != nil {
			return xr
		}
	}
	return clientIP(remoteAddr)
}

func parseSmokeProxyCIDRs(cidrs []string) []*net.IPNet {
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

func ipInNetsSmoke(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// NewUISmokeTestProxy returns an internal proxy for controlled webhook smoke testing.
// It accepts a wrappedHandler that includes the full middleware chain (rate limiting, auth, etc.)
// to ensure smoke tests accurately represent production traffic patterns.
func NewUISmokeTestProxy(wrappedHandler http.Handler, trustedProxyCIDRs []string) http.HandlerFunc {
	trustedNets := parseSmokeProxyCIDRs(trustedProxyCIDRs)
	return func(w http.ResponseWriter, r *http.Request) {
		setUISecurityHeaders(w)
		w.Header().Set("Cache-Control", "no-store")

		if !allowUISmokeRequest(clientIPFromRequestSmoke(r.RemoteAddr, r.Header, trustedNets), time.Now().UTC()) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded for UI smoke tests",
			})
			return
		}

		bodyReader := io.LimitReader(r.Body, uiSmokeMaxBodyBytes)
		defer func() {
			_ = r.Body.Close()
		}()

		var req uiSmokeRequest
		if err := json.NewDecoder(bodyReader).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
			return
		}

		req.Endpoint = strings.TrimSpace(req.Endpoint)
		if !isAllowedSmokeEndpoint(req.Endpoint) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "endpoint not allowed"})
			return
		}
		if len(req.Payload) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload is required"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		internalReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Endpoint, bytes.NewReader(req.Payload))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to build internal request"})
			return
		}
		internalReq.RemoteAddr = r.RemoteAddr
		for k, v := range sanitizeUISmokeHeaders(req.Headers) {
			internalReq.Header.Set(k, v)
		}

		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, internalReq)

		respBody := rr.Body.String()
		if len(respBody) > uiSmokeResponseBodyMaxBytes {
			respBody = respBody[:uiSmokeResponseBodyMaxBytes] + "\n...truncated"
		}

		resp := uiSmokeResponse{
			Endpoint: req.Endpoint,
			Status:   rr.Code,
			Headers: map[string]string{
				"Content-Type": rr.Header().Get("Content-Type"),
				"X-Request-Id": rr.Header().Get("X-Request-Id"),
			},
			Body: respBody,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
