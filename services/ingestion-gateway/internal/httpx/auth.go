package httpx

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
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
			ip := net.ParseIP(ClientIPFromRequest(r, trustedProxyNets))
			if ip == nil {
				if cfg.OnReject != nil {
					cfg.OnReject(r, "admin_cidr_invalid_ip", http.StatusForbidden)
				}
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			for _, network := range nets {
				if network.Contains(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "forbidden", http.StatusForbidden)
			if cfg.OnReject != nil {
				cfg.OnReject(r, "admin_cidr_forbidden", http.StatusForbidden)
			}
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
				if cfg.OnReject != nil {
					cfg.OnReject(r, "admin_jwt_missing", http.StatusUnauthorized)
				}
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			tokenString := strings.TrimSpace(authz[len("Bearer "):])
			if tokenString == "" {
				if cfg.OnReject != nil {
					cfg.OnReject(r, "admin_jwt_missing", http.StatusUnauthorized)
				}
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			if err := validateToken(tokenString, cfg); err != nil {
				if cfg.OnReject != nil {
					cfg.OnReject(r, "admin_jwt_invalid", http.StatusUnauthorized)
				}
				http.Error(w, "invalid token", http.StatusUnauthorized)
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
