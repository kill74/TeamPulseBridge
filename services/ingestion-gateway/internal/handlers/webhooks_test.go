package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/config"
)

type stubPublisher struct {
	calls int
	err   error
}

func (s *stubPublisher) Publish(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	s.calls++
	return s.err
}

func (s *stubPublisher) Close() error {
	return s.err
}

func TestHandleSlackURLVerification(t *testing.T) {
	cfg := config.Config{SlackSigningSecret: "secret"}
	pub := &stubPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewWebhookHandler(cfg, pub, logger, nil)

	body := []byte(`{"type":"url_verification","challenge":"abc123"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := slackSig("secret", ts, string(body))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack", bytes.NewReader(body))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	rr := httptest.NewRecorder()

	h.HandleSlack(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if got["challenge"] != "abc123" {
		t.Fatalf("expected challenge echo, got %q", got["challenge"])
	}
	if pub.calls != 0 {
		t.Fatalf("expected no publish for challenge, got %d", pub.calls)
	}
}

func TestHandleGitHubAccepted(t *testing.T) {
	cfg := config.Config{GitHubWebhookSecret: "gh-secret"}
	pub := &stubPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewWebhookHandler(cfg, pub, logger, nil)

	body := []byte(`{"action":"opened"}`)
	sig := githubSig("gh-secret", body)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	rr := httptest.NewRecorder()

	h.HandleGitHub(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if pub.calls != 1 {
		t.Fatalf("expected one publish call, got %d", pub.calls)
	}
}

func TestHandleGitHubUnauthorized(t *testing.T) {
	cfg := config.Config{GitHubWebhookSecret: "gh-secret"}
	pub := &stubPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewWebhookHandler(cfg, pub, logger, nil)

	body := []byte(`{"action":"opened"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=bad")
	rr := httptest.NewRecorder()

	h.HandleGitHub(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if pub.calls != 0 {
		t.Fatalf("expected no publish calls, got %d", pub.calls)
	}
}

func slackSig(secret, ts, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + ts + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func githubSig(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
