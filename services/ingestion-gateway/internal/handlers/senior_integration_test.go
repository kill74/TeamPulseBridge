package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/platform/resilience"
	"teampulsebridge/services/ingestion-gateway/internal/securityaudit"

	"github.com/stretchr/testify/assert"
)

type PublishCall struct {
	Source  string
	Body    []byte
	Headers map[string]string
}

// PipelineSpy allows us to assert on the full journey of an event.
type PipelineSpy struct {
	mu           sync.Mutex
	published    []PublishCall
	failedEvents []failstore.SaveInput
	security     []securityaudit.SaveInput
}

func (s *PipelineSpy) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.published = append(s.published, PublishCall{Source: source, Body: body, Headers: headers})
	return nil
}

func (s *PipelineSpy) Save(ctx context.Context, in failstore.SaveInput) (failstore.FailedEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failedEvents = append(s.failedEvents, in)
	return failstore.FailedEvent{}, nil
}

// Implement required interface methods for the mock
func (s *PipelineSpy) Close() error { return nil }
func (s *PipelineSpy) HealthCheck(_ context.Context) error { return nil }
func (s *PipelineSpy) GetByID(_ context.Context, _ string) (failstore.FailedEvent, error) { return failstore.FailedEvent{}, nil }
func (s *PipelineSpy) ListRecent(_ context.Context, _ int) ([]failstore.FailedEvent, error) { return nil, nil }

func TestSeniorIntegration_FullIngestionPipeline(t *testing.T) {
	spy := &PipelineSpy{}
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	secret := "webhook-secret"
	cfg := config.Config{
		RequireSecrets:      true,
		SlackSigningSecret:  "secret",
		GitHubWebhookSecret: secret,
		GitLabWebhookToken:  "secret",
		AdminAuthEnabled:    false,
	}

	onSecurityEvent := func(r *http.Request, event SecurityEvent) {
		spy.mu.Lock()
		defer spy.mu.Unlock()
		spy.security = append(spy.security, securityaudit.SaveInput{
			Reason: event.Reason,
			HTTPStatus: event.Status,
		})
	}

	h := NewWebhookHandlerWithDependencies(cfg, spy, logger, nil, nil, spy, onSecurityEvent, nil)

	// Helper to sign GitHub webhooks
	signGitHub := func(secret, body []byte) string {
		mac := hmac.New(sha256.New, secret)
		mac.Write(body)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}

	t.Run("valid payload flow", func(t *testing.T) {
		body := []byte(`{"event":"push","id":"1"}`)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", signGitHub([]byte(secret), body))
		rr := httptest.NewRecorder()

		h.HandleGitHub(rr, req)

		assert.Equal(t, http.StatusAccepted, rr.Code)
		assert.Eventually(t, func() bool {
			spy.mu.Lock()
			defer spy.mu.Unlock()
			return len(spy.published) == 1
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("malicious payload triggers security audit", func(t *testing.T) {
		body := []byte(`{MALICIOUS_JSON_WITHOUT_QUOTES}`)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
		// Use a WRONG secret to trigger unauthorized rejection
		req.Header.Set("X-Hub-Signature-256", signGitHub([]byte("wrong-secret"), body))
		rr := httptest.NewRecorder()

		h.HandleGitHub(rr, req)

		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		
		spy.mu.Lock()
		assert.GreaterOrEqual(t, len(spy.security), 1, "security event should have been recorded")
		spy.mu.Unlock()
	})
}

func TestSeniorIntegration_QueueSaturation(t *testing.T) {
	spy := &PipelineSpy{}
	fullPub := &fullPublisher{}
	secret := "webhook-secret"
	
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
	cfg := config.Config{
		RequireSecrets:      true,
		GitHubWebhookSecret: secret,
	}
	
	h := NewWebhookHandlerWithDependencies(cfg, fullPub, logger, nil, nil, spy, nil, nil)

	// Helper to sign GitHub webhooks
	signGitHub := func(secret, body []byte) string {
		mac := hmac.New(sha256.New, secret)
		mac.Write(body)
		return "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}

	body := []byte(`{"event":"push","id":"sat"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", signGitHub([]byte(secret), body))
	rr := httptest.NewRecorder()

	h.HandleGitHub(rr, req)

	// Circuit breakers typically map to Internal Server Error 500 when they trigger
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	
	assert.Eventually(t, func() bool {
		spy.mu.Lock()
		defer spy.mu.Unlock()
		return len(spy.failedEvents) == 1
	}, time.Second, 10*time.Millisecond)
}

type fullPublisher struct{}
func (f *fullPublisher) Publish(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	return resilience.ErrCircuitOpen
}
func (f *fullPublisher) Close() error { return nil }
func (f *fullPublisher) HealthCheck(_ context.Context) error { return nil }
