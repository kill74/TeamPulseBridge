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

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/dedup"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
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

type captureFailedStore struct {
	calls int
	last  failstore.SaveInput
}

func (s *captureFailedStore) Save(_ context.Context, in failstore.SaveInput) (failstore.FailedEvent, error) {
	s.calls++
	s.last = in
	return failstore.FailedEvent{EventID: in.EventID}, nil
}

func (s *captureFailedStore) GetByID(_ context.Context, _ string) (failstore.FailedEvent, error) {
	return failstore.FailedEvent{}, failstore.ErrNotFound
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
	assertErrorCode(t, rr.Body.Bytes(), apperr.CodeUnauthorized)
	if pub.calls != 0 {
		t.Fatalf("expected no publish calls, got %d", pub.calls)
	}
}

func TestHandleGitHubMethodNotAllowed(t *testing.T) {
	cfg := config.Config{GitHubWebhookSecret: "gh-secret"}
	pub := &stubPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewWebhookHandler(cfg, pub, logger, nil)

	req := httptest.NewRequest(http.MethodGet, "/webhooks/github", nil)
	rr := httptest.NewRecorder()

	h.HandleGitHub(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if allow := rr.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header %q, got %q", http.MethodPost, allow)
	}
	assertErrorCode(t, rr.Body.Bytes(), apperr.CodeMethodNotAllowed)
	if pub.calls != 0 {
		t.Fatalf("expected no publish calls, got %d", pub.calls)
	}
}

func TestHandleGitHubPayloadTooLarge(t *testing.T) {
	cfg := config.Config{GitHubWebhookSecret: "gh-secret"}
	pub := &stubPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewWebhookHandler(cfg, pub, logger, nil)

	body := bytes.Repeat([]byte("a"), maxBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.HandleGitHub(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
	assertErrorCode(t, rr.Body.Bytes(), apperr.CodePayloadTooLarge)
	if pub.calls != 0 {
		t.Fatalf("expected no publish calls, got %d", pub.calls)
	}
}

func TestHandleGitHubDuplicateIgnored(t *testing.T) {
	cfg := config.Config{GitHubWebhookSecret: "gh-secret"}
	pub := &stubPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewWebhookHandlerWithDependencies(cfg, pub, logger, nil, dedup.NewMemory(true, time.Minute), nil)

	body := []byte(`{"action":"opened"}`)
	sig := githubSig("gh-secret", body)

	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req1.Header.Set("X-Hub-Signature-256", sig)
	req1.Header.Set("X-GitHub-Delivery", "delivery-123")
	rr1 := httptest.NewRecorder()
	h.HandleGitHub(rr1, req1)
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("expected first response 202, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req2.Header.Set("X-Hub-Signature-256", sig)
	req2.Header.Set("X-GitHub-Delivery", "delivery-123")
	rr2 := httptest.NewRecorder()
	h.HandleGitHub(rr2, req2)
	if rr2.Code != http.StatusAccepted {
		t.Fatalf("expected second response 202, got %d", rr2.Code)
	}

	if pub.calls != 1 {
		t.Fatalf("expected one publish call due to dedup, got %d", pub.calls)
	}
}

func TestHandleGitHubQueueFailurePersistsFailedEvent(t *testing.T) {
	cfg := config.Config{GitHubWebhookSecret: "gh-secret"}
	pub := &stubPublisher{err: queue.ErrQueueFull}
	store := &captureFailedStore{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewWebhookHandlerWithDependencies(cfg, pub, logger, nil, dedup.NewMemory(true, time.Minute), store)

	body := []byte(`{"action":"opened"}`)
	sig := githubSig("gh-secret", body)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Delivery", "delivery-queue-fail")
	rr := httptest.NewRecorder()
	h.HandleGitHub(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	assertErrorCode(t, rr.Body.Bytes(), apperr.CodeQueueFull)
	if store.calls != 1 {
		t.Fatalf("expected failed event persisted once, got %d", store.calls)
	}
	if store.last.EventID != "delivery-queue-fail" {
		t.Fatalf("expected event id delivery-queue-fail, got %q", store.last.EventID)
	}
	if store.last.Reason != string(apperr.CodeQueueFull) {
		t.Fatalf("expected reason %q, got %q", apperr.CodeQueueFull, store.last.Reason)
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

func assertErrorCode(t *testing.T, body []byte, expected apperr.Code) {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("expected JSON error payload, got error: %v body=%q", err, body)
	}
	if payload.Error.Code != string(expected) {
		t.Fatalf("expected error code %q, got %q", expected, payload.Error.Code)
	}
}
