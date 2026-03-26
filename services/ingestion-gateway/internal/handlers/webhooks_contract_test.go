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
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/config"
)

type capturePublisher struct {
	calls []captureCall
}

type captureCall struct {
	source  string
	body    []byte
	headers map[string]string
}

func (c *capturePublisher) Publish(_ context.Context, source string, body []byte, headers map[string]string) error {
	copiedBody := make([]byte, len(body))
	copy(copiedBody, body)
	copiedHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		copiedHeaders[k] = v
	}
	c.calls = append(c.calls, captureCall{source: source, body: copiedBody, headers: copiedHeaders})
	return nil
}

func TestWebhookPayloadContracts_PublishRawCompatiblePayloads(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metricsFn := func(context.Context, string, int) {}

	tests := []struct {
		name       string
		source     string
		payload    string
		newRequest func(t *testing.T, payload []byte) *http.Request
		handle     func(h *WebhookHandler, w http.ResponseWriter, r *http.Request)
		cfg        config.Config
	}{
		{
			name:    "slack event_callback",
			source:  "slack",
			payload: "slack_event_callback.json",
			cfg:     config.Config{SlackSigningSecret: "test-slack-secret"},
			newRequest: func(t *testing.T, payload []byte) *http.Request {
				t.Helper()
				ts := strconv.FormatInt(time.Now().Unix(), 10)
				sig := slackSignature("test-slack-secret", ts, payload)
				req := httptest.NewRequest(http.MethodPost, "/webhooks/slack", bytes.NewReader(payload))
				req.Header.Set("X-Slack-Request-Timestamp", ts)
				req.Header.Set("X-Slack-Signature", sig)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "contract-test/slack")
				return req
			},
			handle: func(h *WebhookHandler, w http.ResponseWriter, r *http.Request) { h.HandleSlack(w, r) },
		},
		{
			name:    "github pull_request",
			source:  "github",
			payload: "github_pull_request_opened.json",
			cfg:     config.Config{GitHubWebhookSecret: "test-github-secret"},
			newRequest: func(t *testing.T, payload []byte) *http.Request {
				t.Helper()
				sig := githubSignature("test-github-secret", payload)
				req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
				req.Header.Set("X-Hub-Signature-256", sig)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "contract-test/github")
				return req
			},
			handle: func(h *WebhookHandler, w http.ResponseWriter, r *http.Request) { h.HandleGitHub(w, r) },
		},
		{
			name:    "gitlab merge_request",
			source:  "gitlab",
			payload: "gitlab_merge_request.json",
			cfg:     config.Config{GitLabWebhookToken: "test-gitlab-token"},
			newRequest: func(t *testing.T, payload []byte) *http.Request {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader(payload))
				req.Header.Set("X-Gitlab-Token", "test-gitlab-token")
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "contract-test/gitlab")
				return req
			},
			handle: func(h *WebhookHandler, w http.ResponseWriter, r *http.Request) { h.HandleGitLab(w, r) },
		},
		{
			name:    "teams change notification",
			source:  "teams",
			payload: "teams_change_notification.json",
			cfg:     config.Config{TeamsClientState: "test-teams-state"},
			newRequest: func(t *testing.T, payload []byte) *http.Request {
				t.Helper()
				req := httptest.NewRequest(http.MethodPost, "/webhooks/teams", bytes.NewReader(payload))
				req.Header.Set("X-Client-State", "test-teams-state")
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "contract-test/teams")
				return req
			},
			handle: func(h *WebhookHandler, w http.ResponseWriter, r *http.Request) { h.HandleTeams(w, r) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pub := &capturePublisher{}
			h := NewWebhookHandler(tc.cfg, pub, logger, metricsFn)

			payload := mustReadFixture(t, tc.payload)
			req := tc.newRequest(t, payload)
			rr := httptest.NewRecorder()

			tc.handle(h, rr, req)

			if rr.Code != http.StatusAccepted {
				t.Fatalf("expected status 202, got %d, body=%s", rr.Code, rr.Body.String())
			}
			if len(pub.calls) != 1 {
				t.Fatalf("expected 1 publish call, got %d", len(pub.calls))
			}
			call := pub.calls[0]
			if call.source != tc.source {
				t.Fatalf("expected source %q, got %q", tc.source, call.source)
			}
			assertJSONEqual(t, payload, call.body)
			if call.headers["Content-Type"] != "application/json" {
				t.Fatalf("expected content-type application/json, got %q", call.headers["Content-Type"])
			}
			if call.headers["User-Agent"] == "" {
				t.Fatal("expected user-agent header to be propagated")
			}
		})
	}
}

func TestWebhookPayloadContracts_SlackURLVerification_NoPublish(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pub := &capturePublisher{}
	h := NewWebhookHandler(config.Config{SlackSigningSecret: "test-slack-secret"}, pub, logger, nil)

	payload := []byte(`{"type":"url_verification","challenge":"contract-challenge"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := slackSignature("test-slack-secret", ts, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/slack", bytes.NewReader(payload))
	req.Header.Set("X-Slack-Request-Timestamp", ts)
	req.Header.Set("X-Slack-Signature", sig)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	h.HandleSlack(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if len(pub.calls) != 0 {
		t.Fatalf("expected 0 publish calls, got %d", len(pub.calls))
	}
}

func mustReadFixture(t *testing.T, fileName string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "contracts", fileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
}

func slackSignature(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + ts + ":" + string(body)))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func githubSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func assertJSONEqual(t *testing.T, expected, actual []byte) {
	t.Helper()
	var lhs any
	var rhs any
	if err := json.Unmarshal(expected, &lhs); err != nil {
		t.Fatalf("unmarshal expected payload: %v", err)
	}
	if err := json.Unmarshal(actual, &rhs); err != nil {
		t.Fatalf("unmarshal actual payload: %v", err)
	}
	if !deepEqualJSON(lhs, rhs) {
		t.Fatalf("payload mismatch\nexpected: %s\nactual:   %s", string(expected), string(actual))
	}
}

func deepEqualJSON(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}
