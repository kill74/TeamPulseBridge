package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/testhelpers/fixturecatalog"
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

func (c *capturePublisher) Close() error {
	return nil
}

func TestWebhookFixtureCatalog_IsVersionedAndComplete(t *testing.T) {
	catalog, err := fixturecatalog.Load()
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}
	if catalog.CatalogVersion != 1 {
		t.Fatalf("expected catalog_version=1, got %d", catalog.CatalogVersion)
	}

	files, err := fixturecatalog.ListFixtureFiles()
	if err != nil {
		t.Fatalf("list fixture files: %v", err)
	}

	catalogPaths := make(map[string]struct{}, len(catalog.Fixtures))
	for _, fixture := range catalog.Fixtures {
		catalogPaths[fixture.Path] = struct{}{}
	}
	if len(files) != len(catalogPaths) {
		t.Fatalf("fixture catalog/file count mismatch: files=%d catalog=%d", len(files), len(catalogPaths))
	}
	for _, file := range files {
		if _, ok := catalogPaths[file]; !ok {
			t.Fatalf("fixture file %q is not registered in catalog-v1.json", file)
		}
	}
}

func TestWebhookPayloadContracts_FromCatalog(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metricsFn := func(context.Context, string, int) {}
	catalog, err := fixturecatalog.Load()
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}

	for _, tc := range catalog.Fixtures {
		t.Run(tc.ID, func(t *testing.T) {
			pub := &capturePublisher{}
			h := NewWebhookHandler(contractConfig(tc.Provider), pub, logger, metricsFn)

			payload, err := fixturecatalog.ReadFixture(tc)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			req := newCatalogRequest(t, tc, payload)
			rr := httptest.NewRecorder()

			handleCatalogFixture(h, tc.Provider, rr, req)

			if rr.Code != tc.ExpectedStatus {
				t.Fatalf("expected status %d, got %d, body=%s", tc.ExpectedStatus, rr.Code, rr.Body.String())
			}
			if !tc.Publish {
				if len(pub.calls) != 0 {
					t.Fatalf("expected no publish calls, got %d", len(pub.calls))
				}
				if tc.Provider == "slack" && tc.EventFamily == "url_verification" {
					var response map[string]string
					if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
						t.Fatalf("unmarshal url verification response: %v", err)
					}
					if response["challenge"] == "" {
						t.Fatal("expected slack challenge echo in response")
					}
				}
				return
			}
			if len(pub.calls) != 1 {
				t.Fatalf("expected 1 publish call, got %d", len(pub.calls))
			}
			call := pub.calls[0]
			if call.source != tc.Provider {
				t.Fatalf("expected source %q, got %q", tc.Provider, call.source)
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

func TestWebhookCompatibilityMatrix_CoversCatalogFamilies(t *testing.T) {
	catalog, err := fixturecatalog.Load()
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}
	doc, err := os.ReadFile(fixturecatalog.CompatibilityMatrixPath())
	if err != nil {
		t.Fatalf("read compatibility matrix: %v", err)
	}
	matrix := strings.ToLower(string(doc))
	for _, family := range catalog.ProviderFamilies() {
		parts := strings.SplitN(family, "|", 2)
		rowMarker := fmt.Sprintf("| %s | %s |", parts[0], parts[1])
		if !strings.Contains(matrix, rowMarker) {
			t.Fatalf("compatibility matrix is missing provider/event family row %q", rowMarker)
		}
	}
}

func contractConfig(provider string) config.Config {
	switch provider {
	case "slack":
		return config.Config{SlackSigningSecret: "test-slack-secret"}
	case "github":
		return config.Config{GitHubWebhookSecret: "test-github-secret"}
	case "gitlab":
		return config.Config{GitLabWebhookToken: "test-gitlab-token"}
	case "teams":
		return config.Config{TeamsClientState: "test-teams-state"}
	default:
		return config.Config{}
	}
}

func handleCatalogFixture(h *WebhookHandler, provider string, w http.ResponseWriter, r *http.Request) {
	switch provider {
	case "slack":
		h.HandleSlack(w, r)
	case "github":
		h.HandleGitHub(w, r)
	case "gitlab":
		h.HandleGitLab(w, r)
	case "teams":
		h.HandleTeams(w, r)
	default:
		panic("unexpected provider: " + provider)
	}
}

func newCatalogRequest(t *testing.T, fixture fixturecatalog.Fixture, payload []byte) *http.Request {
	t.Helper()
	userAgent := "contract-test/" + fixture.ID
	switch fixture.Provider {
	case "slack":
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		sig := slackSignature("test-slack-secret", ts, payload)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/slack", bytes.NewReader(payload))
		req.Header.Set("X-Slack-Request-Timestamp", ts)
		req.Header.Set("X-Slack-Signature", sig)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)
		return req
	case "github":
		sig := githubSignature("test-github-secret", payload)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", sig)
		req.Header.Set("X-GitHub-Delivery", fixture.ID)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)
		return req
	case "gitlab":
		req := httptest.NewRequest(http.MethodPost, "/webhooks/gitlab", bytes.NewReader(payload))
		req.Header.Set("X-Gitlab-Token", "test-gitlab-token")
		req.Header.Set("X-Request-Id", fixture.ID)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)
		return req
	case "teams":
		req := httptest.NewRequest(http.MethodPost, "/webhooks/teams", bytes.NewReader(payload))
		req.Header.Set("X-Client-State", "test-teams-state")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", userAgent)
		return req
	default:
		t.Fatalf("unsupported provider %q", fixture.Provider)
		return nil
	}
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
