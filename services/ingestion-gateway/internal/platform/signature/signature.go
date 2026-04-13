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

	base := "v0:" + timestamp + ":" + body
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expected), []byte(providedSignature)) != 1 {
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
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
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
	if subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) != 1 {
		return fmt.Errorf("invalid teams client state")
	}
	return nil
}
