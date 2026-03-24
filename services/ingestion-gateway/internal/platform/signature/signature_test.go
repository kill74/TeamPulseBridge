package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestValidateSlack_OK(t *testing.T) {
	secret := "top-secret"
	ts := "1711276800"
	body := "{\"type\":\"event_callback\"}"

	base := "v0:" + ts + ":" + body
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	now := time.Unix(1711276800, 0)
	if err := ValidateSlack(secret, ts, body, sig, now, 5*time.Minute); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestValidateSlack_FailsWithSkew(t *testing.T) {
	secret := "top-secret"
	ts := "1711276800"
	body := "{}"
	base := "v0:" + ts + ":" + body
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	now := time.Unix(1711276800, 0).Add(10 * time.Minute)
	if err := ValidateSlack(secret, ts, body, sig, now, 5*time.Minute); err == nil {
		t.Fatal("expected skew validation error")
	}
}

func TestValidateGitHub_OK(t *testing.T) {
	secret := "gh-secret"
	body := []byte("payload")

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if err := ValidateGitHub(secret, body, sig); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestValidateGitLab_OK(t *testing.T) {
	if err := ValidateGitLab("token", "token"); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}

func TestValidateTeamsClientState_OK(t *testing.T) {
	if err := ValidateTeamsClientState("state-123", "state-123"); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
}
