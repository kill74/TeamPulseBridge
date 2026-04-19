package httpx

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
)

type JWTConfig struct {
	Enabled  bool
	Issuer   string
	Audience string
	Secret   string
	OnReject func(r *http.Request, reason string, status int)
}

type AdminCIDRConfig struct {
	Enabled           bool
	CIDRs             []string
	TrustedProxyCIDRs []string
	OnReject          func(r *http.Request, reason string, status int)
}

func RequireAdminCIDRAllowlist(cfg AdminCIDRConfig) Middleware {
	if !cfg.Enabled || len(cfg.CIDRs) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	nets := make([]*net.IPNet, 0, len(cfg.CIDRs))
	for _, cidr := range cfg.CIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(cidr))
		if err != nil {
			continue
		}
		nets = append(nets, network)
	}
	if len(nets) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	trustedProxyNets := parseCIDRs(cfg.TrustedProxyCIDRs)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isAdminPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			clientIP := ClientIPFromRequest(r, trustedProxyNets)
			ip := net.ParseIP(clientIP)
			if ip == nil {
				rejectSecurity(w, r, http.StatusForbidden, "admin_cidr_invalid_ip", cfg.OnReject, apperr.New(
					"httpx.RequireAdminCIDRAllowlist",
					apperr.CodeForbidden,
					"forbidden",
					fmt.Errorf("invalid client ip: %s", clientIP),
				))
				return
			}
			for _, network := range nets {
				if network.Contains(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}
			rejectSecurity(w, r, http.StatusForbidden, "admin_cidr_forbidden", cfg.OnReject, apperr.New(
				"httpx.RequireAdminCIDRAllowlist",
				apperr.CodeForbidden,
				"forbidden",
				fmt.Errorf("ip %s outside admin allowlist", ip.String()),
			))
		})
	}
}

func RequireAdminJWT(cfg JWTConfig) Middleware {
	return func(next http.Handler) http.Handler {
		if !cfg.Enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isAdminPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				rejectSecurity(w, r, http.StatusUnauthorized, "admin_jwt_missing", cfg.OnReject, apperr.New(
					"httpx.RequireAdminJWT",
					apperr.CodeMissingBearerToken,
					"missing bearer token",
					nil,
				))
				return
			}
			tokenString := strings.TrimSpace(authz[len("Bearer "):])
			if tokenString == "" {
				rejectSecurity(w, r, http.StatusUnauthorized, "admin_jwt_missing", cfg.OnReject, apperr.New(
					"httpx.RequireAdminJWT",
					apperr.CodeMissingBearerToken,
					"missing bearer token",
					nil,
				))
				return
			}

			if err := validateToken(tokenString, cfg); err != nil {
				rejectSecurity(w, r, http.StatusUnauthorized, "admin_jwt_invalid", cfg.OnReject, apperr.New(
					"httpx.RequireAdminJWT",
					apperr.CodeInvalidToken,
					"invalid token",
					err,
				))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isAdminPath(path string) bool {
	return strings.HasPrefix(path, "/admin")
}

func validateToken(tokenString string, cfg JWTConfig) error {
	token, err := jwt.Parse(tokenString,
		func(t *jwt.Token) (interface{}, error) {
			if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method %s", t.Method.Alg())
			}
			return []byte(cfg.Secret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
	)
	if err != nil {
		return err
	}
	if !token.Valid {
		return fmt.Errorf("token invalid")
	}
	return nil
}

func rejectSecurity(w http.ResponseWriter, r *http.Request, status int, reason string, onReject func(r *http.Request, reason string, status int), err *apperr.Error) {
	if onReject != nil {
		onReject(r, reason, status)
	}
	WriteError(w, r.Context(), status, err, nil)
}
