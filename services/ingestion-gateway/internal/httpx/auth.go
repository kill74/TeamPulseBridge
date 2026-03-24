package httpx

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	Enabled  bool
	Issuer   string
	Audience string
	Secret   string
}

func RequireAdminJWT(cfg JWTConfig) Middleware {
	return func(next http.Handler) http.Handler {
		if !cfg.Enabled {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/admin") && r.URL.Path != "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			if authz == "" || !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			tokenString := strings.TrimSpace(authz[len("Bearer "):])
			if tokenString == "" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			if err := validateToken(tokenString, cfg); err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
