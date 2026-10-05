package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func ValidateSlack(secret, timestamp, body, providedSignature string, now time.Time, maxSkew time.Duration) error {
	if secret == "" {
		return fmt.Errorf("slack signing secret is not configured")
	}
	if timestamp == "" || providedSignature == "" {
		return fmt.Errorf("missing slack timestamp or signature")
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid slack timestamp: %w", err)
	}

	t := time.Unix(ts, 0)
	if now.Sub(t) > maxSkew || t.Sub(now) > maxSkew {
		return fmt.Errorf("slack request timestamp outside allowed skew")
	}

	// Write components incrementally to avoid building a ~1MiB concat string.
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:"))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(body))
	expectedMAC := mac.Sum(nil)

	trimmed := strings.TrimSpace(providedSignature)
	if !strings.HasPrefix(trimmed, "v0=") {
		return fmt.Errorf("invalid slack signature")
	}
	providedBytes, err := hex.DecodeString(strings.TrimPrefix(trimmed, "v0="))
	if err != nil {
		return fmt.Errorf("invalid slack signature")
	}
	if !hmac.Equal(expectedMAC, providedBytes) {
		return fmt.Errorf("invalid slack signature")
	}
	return nil
}

func ValidateGitHub(secret string, body []byte, provided string) error {
	if secret == "" {
		return fmt.Errorf("github webhook secret is not configured")
	}
	if provided == "" {
		return fmt.Errorf("missing github signature")
	}
	if !strings.HasPrefix(provided, "sha256=") {
		return fmt.Errorf("github signature format is invalid")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expectedMAC := mac.Sum(nil)

	providedBytes, err := hex.DecodeString(strings.TrimSpace(strings.TrimPrefix(provided, "sha256=")))
	if err != nil {
		return fmt.Errorf("invalid github signature")
	}
	if !hmac.Equal(expectedMAC, providedBytes) {
		return fmt.Errorf("invalid github signature")
	}
	return nil
}

func ValidateGitLab(token, provided string) error {
	if token == "" {
		return fmt.Errorf("gitlab webhook token is not configured")
	}
	if provided == "" {
		return fmt.Errorf("missing gitlab webhook token")
	}

	// Constant-time compare without hashing tiny tokens twice.
	if len(token) != len(provided) {
		// Compare same-length dummy buffers to keep timing stable, then fail.
		dummy := make([]byte, len(token))
		_ = subtle.ConstantTimeCompare([]byte(token), dummy)
		return fmt.Errorf("invalid gitlab token")
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(provided)) != 1 {
		return fmt.Errorf("invalid gitlab token")
	}
	return nil
}

func ValidateTeamsClientState(expected, provided string) error {
	if expected == "" {
		return fmt.Errorf("teams client state is not configured")
	}
	if provided == "" {
		return fmt.Errorf("missing teams client state")
	}

	if len(expected) != len(provided) {
		dummy := make([]byte, len(expected))
		_ = subtle.ConstantTimeCompare([]byte(expected), dummy)
		return fmt.Errorf("invalid teams client state")
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		return fmt.Errorf("invalid teams client state")
	}
	return nil
}
