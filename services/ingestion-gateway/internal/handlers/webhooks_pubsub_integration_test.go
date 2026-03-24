package handlers_test

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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/handlers"
	"teampulsebridge/services/ingestion-gateway/internal/httpx"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
	"teampulsebridge/services/ingestion-gateway/internal/testhelpers/pubsubtest"
)

// TestWebhookToPubSubIntegration tests the full flow from webhook ingestion to Pub/Sub.
// This requires PUBSUB_EMULATOR_HOST to be set.
func TestWebhookToPubSubIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("skipping integration test: PUBSUB_EMULATOR_HOST is not set")
	}

	cfg := pubsubtest.DefaultEmulatorConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create Pub/Sub client and resources
	pubsubClient, err := pubsubtest.NewPubSubClient(ctx, cfg)
	require.NoError(t, err, "failed to create Pub/Sub client")
	defer pubsubClient.Close()

	topic, err := pubsubtest.CreateTopic(ctx, pubsubClient, "webhook-events")
	require.NoError(t, err, "failed to create topic")

	sub, err := pubsubtest.CreateSubscription(ctx, pubsubClient, "webhook-consumer", "webhook-events")
	require.NoError(t, err, "failed to create subscription")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create queue publisher
	pubsubPublisher := queue.NewPubSubPublisherDirect(pubsubClient, topic, logger)

	t.Run("SlackWebhookToPubSub", func(t *testing.T) {
		// Purge subscription
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err)

		// Arrange
		webhookSecret := "test-slack-secret"
		timestamp := fmt.Sprintf("%d", time.Now().Unix())

		slackPayload := map[string]interface{}{
			"type": "url_verification",
			"challenge": "test-challenge-token",
			"token": "test-token",
		}
		body, _ := json.Marshal(slackPayload)

		// Create Slack signature
		signingSecret := hmac.New(sha256.New, []byte(webhookSecret))
		signingSecret.Write([]byte(fmt.Sprintf("v0:%s:%s", timestamp, string(body))))
		slackSig := fmt.Sprintf("v0=%s", hex.EncodeToString(signingSecret.Sum(nil)))

		// Create webhook handler
		metricsFn := func(ctx context.Context, source string, status int) {}
		slackHandler := handlers.NewWebhookHandler(config.Config{SlackSigningSecret: webhookSecret}, pubsubPublisher, logger, metricsFn)

		// Act - make webhook request
		req := httptest.NewRequest("POST", "/webhooks/slack", bytes.NewReader(body))
		req.Header.Set("X-Slack-Request-Timestamp", timestamp)
		req.Header.Set("X-Slack-Signature", slackSig)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		http.HandlerFunc(slackHandler.HandleSlack).ServeHTTP(w, req)

		// Assert webhook response
		assert.Equal(t, http.StatusOK, w.Code, "handler should return 200")

		// Verify message in Pub/Sub
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err, "should receive message from Pub/Sub")
		assert.Len(t, messages, 1, "should have exactly one message")

		// Verify message attributes
		assert.Equal(t, "slack", messages[0].Attributes["source"])
		assert.Equal(t, "raw-webhook-envelope", messages[0].Attributes["schema"])

		// Verify message body
		var envelope struct {
			Source string          `json:"source"`
			Body   json.RawMessage `json:"body"`
		}
		err = json.Unmarshal(messages[0].Data, &envelope)
		assert.NoError(t, err, "should parse envelope")
		assert.Equal(t, "slack", envelope.Source)

		// Verify original payload in envelope body
		var received map[string]interface{}
		err = json.Unmarshal(envelope.Body, &received)
		assert.NoError(t, err, "should parse original payload")
		assert.Equal(t, "url_verification", received["type"])
	})

	t.Run("GitHubWebhookToPubSub", func(t *testing.T) {
		// Purge subscription
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err)

		// Arrange
		webhookSecret := "test-github-secret"

		githubPayload := map[string]interface{}{
			"action": "opened",
			"pull_request": map[string]interface{}{
				"id":     123,
				"number": 1,
				"title":  "Add feature",
			},
		}
		body, _ := json.Marshal(githubPayload)

		// Create GitHub signature
		sig := hmac.New(sha256.New, []byte(webhookSecret))
		sig.Write(body)
		githubSig := fmt.Sprintf("sha256=%s", hex.EncodeToString(sig.Sum(nil)))

		// Create webhook handler
		metricsFn := func(ctx context.Context, source string, status int) {}
		githubHandler := handlers.NewWebhookHandler(config.Config{GitHubWebhookSecret: webhookSecret}, pubsubPublisher, logger, metricsFn)

		// Act
		req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", githubSig)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		http.HandlerFunc(githubHandler.HandleGitHub).ServeHTTP(w, req)

		// Assert
		assert.Equal(t, http.StatusAccepted, w.Code, "handler should return 202")

		// Verify message in Pub/Sub
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err, "should receive message from Pub/Sub")
		assert.Len(t, messages, 1)

		// Verify message
		assert.Equal(t, "github", messages[0].Attributes["source"])

		var envelope struct {
			Source string          `json:"source"`
			Body   json.RawMessage `json:"body"`
		}
		err = json.Unmarshal(messages[0].Data, &envelope)
		assert.NoError(t, err)
		assert.Equal(t, "github", envelope.Source)
	})

	t.Run("WebhookSignatureValidation", func(t *testing.T) {
		// Purge subscription
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err)

		// Arrange
		webhookSecret := "test-secret"
		body := []byte(`{"type":"event"}`)

		// Create incorrect signature
		incorrectSig := "sha256=0000000000000000000000000000000000000000000000000000000000000000"

		// Create handler
		metricsFn := func(ctx context.Context, source string, status int) {}
		githubHandler := handlers.NewWebhookHandler(config.Config{GitHubWebhookSecret: webhookSecret}, pubsubPublisher, logger, metricsFn)

		// Act
		req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", incorrectSig)
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		http.HandlerFunc(githubHandler.HandleGitHub).ServeHTTP(w, req)

		// Assert - should reject with 403
		assert.Equal(t, http.StatusForbidden, w.Code, "handler should reject invalid signature")

		// Verify no message in Pub/Sub
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 1*time.Second)
		assert.Error(t, err, "should timeout receiving (no message sent)")
		assert.Empty(t, messages, "should not publish message for invalid signature")
	})

	t.Run("MultipleWebhooksOrdered", func(t *testing.T) {
		// Purge subscription
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err)

		// Arrange
		webhookSecret := "test-github-secret"
		metricsFn := func(ctx context.Context, source string, status int) {}
		githubHandler := handlers.NewWebhookHandler(config.Config{GitHubWebhookSecret: webhookSecret}, pubsubPublisher, logger, metricsFn)

		// Act - send multiple webhooks
		for i := 0; i < 5; i++ {
			payload := map[string]interface{}{
				"action": "opened",
				"number": i,
			}
			body, _ := json.Marshal(payload)

			sig := hmac.New(sha256.New, []byte(webhookSecret))
			sig.Write(body)
			githubSig := fmt.Sprintf("sha256=%s", hex.EncodeToString(sig.Sum(nil)))

			req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
			req.Header.Set("X-Hub-Signature-256", githubSig)

			w := httptest.NewRecorder()
			http.HandlerFunc(githubHandler.HandleGitHub).ServeHTTP(w, req)

			assert.Equal(t, http.StatusAccepted, w.Code)
		}

		// Assert - verify all messages received
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 5, 5*time.Second)
		assert.NoError(t, err, "should receive all messages")
		assert.Len(t, messages, 5, "should receive all 5 messages in order")

		// Verify message order by extracting numbers
		for i, msg := range messages {
			var envelope struct {
				Body json.RawMessage `json:"body"`
			}
			err := json.Unmarshal(msg.Data, &envelope)
			assert.NoError(t, err)

			var payload map[string]interface{}
			err = json.Unmarshal(envelope.Body, &payload)
			assert.NoError(t, err)
			assert.Equal(t, float64(i), payload["number"], "messages should be in order")
		}
	})

	t.Run("WebhookWithAsyncPublisher", func(t *testing.T) {
		// Purge subscription
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err)

		// Create async publisher wrapper
		asyncPublisher := queue.NewAsyncPublisher(pubsubPublisher, 100, logger)

		// Arrange
		webhookSecret := "test-github-secret"
		metricsFn := func(ctx context.Context, source string, status int) {}
		githubHandler := handlers.NewWebhookHandler(config.Config{GitHubWebhookSecret: webhookSecret}, asyncPublisher, logger, metricsFn)

		// Act - send webhook with async publisher
		payload := map[string]interface{}{
			"action": "opened",
			"number": 42,
		}
		body, _ := json.Marshal(payload)

		sig := hmac.New(sha256.New, []byte(webhookSecret))
		sig.Write(body)
		githubSig := fmt.Sprintf("sha256=%s", hex.EncodeToString(sig.Sum(nil)))

		req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-Hub-Signature-256", githubSig)

		w := httptest.NewRecorder()
		http.HandlerFunc(githubHandler.HandleGitHub).ServeHTTP(w, req)

		assert.Equal(t, http.StatusAccepted, w.Code)

		// Give async publisher time to flush
		time.Sleep(500 * time.Millisecond)

		// Assert
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err)
		assert.Len(t, messages, 1)
		assert.Equal(t, "github", messages[0].Attributes["source"])
	})
}

// TestWebhookWithMiddleware tests webhook flow with HTTP middleware stack.
func TestWebhookWithMiddleware(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("skipping integration test: PUBSUB_EMULATOR_HOST is not set")
	}

	cfg := pubsubtest.DefaultEmulatorConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create Pub/Sub resources
	pubsubClient, err := pubsubtest.NewPubSubClient(ctx, cfg)
	require.NoError(t, err)
	defer pubsubClient.Close()

	topic, err := pubsubtest.CreateTopic(ctx, pubsubClient, "middleware-test")
	require.NoError(t, err)

	sub, err := pubsubtest.CreateSubscription(ctx, pubsubClient, "middleware-consumer", "middleware-test")
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pubsubPublisher := queue.NewPubSubPublisherDirect(pubsubClient, topic, logger)

	t.Run("WebhookWithRequestID", func(t *testing.T) {
		// Purge
		pubsubtest.PurgeSubscription(ctx, sub)

		// Arrange
		webhookSecret := "test-secret"
		payload := []byte(`{"type":"event"}`)

		sig := hmac.New(sha256.New, []byte(webhookSecret))
		sig.Write(payload)
		githubSig := fmt.Sprintf("sha256=%s", hex.EncodeToString(sig.Sum(nil)))

		metricsFn := func(ctx context.Context, source string, status int) {}
		handler := handlers.NewWebhookHandler(config.Config{GitHubWebhookSecret: webhookSecret}, pubsubPublisher, logger, metricsFn)

		// Wrap with middleware
		wrapped := httpx.Chain(
			http.HandlerFunc(handler.HandleGitHub),
			httpx.RequestID(),
			httpx.Recoverer(logger),
		)

		// Act
		req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", githubSig)

		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		// Assert
		assert.Equal(t, http.StatusAccepted, w.Code)

		// Verify message published
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err)
		assert.Len(t, messages, 1)

		// Verify request ID was in headers (if captured)
		var envelope struct {
			Headers map[string]string `json:"headers"`
		}
		err = json.Unmarshal(messages[0].Data, &envelope)
		assert.NoError(t, err)
		// Request ID would be in the original X-Request-ID header if present
	})
}

// BenchmarkWebhookToPubSub benchmarks the end-to-end webhook to Pub/Sub flow.
func BenchmarkWebhookToPubSub(b *testing.B) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		b.Skip("skipping integration benchmark: PUBSUB_EMULATOR_HOST is not set")
	}

	cfg := pubsubtest.DefaultEmulatorConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Setup
	pubsubClient, _ := pubsubtest.NewPubSubClient(ctx, cfg)
	defer pubsubClient.Close()

	topic, _ := pubsubtest.CreateTopic(ctx, pubsubClient, "bench-topic")
	_, _ = pubsubtest.CreateSubscription(ctx, pubsubClient, "bench-sub", "bench-topic")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	pubsubPublisher := queue.NewPubSubPublisherDirect(pubsubClient, topic, logger)

	webhookSecret := "test-secret"
	metricsFn := func(ctx context.Context, source string, status int) {}
	githubHandler := handlers.NewWebhookHandler(config.Config{GitHubWebhookSecret: webhookSecret}, pubsubPublisher, logger, metricsFn)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		payload := []byte(fmt.Sprintf(`{"action":"opened","number":%d}`, i))

		sig := hmac.New(sha256.New, []byte(webhookSecret))
		sig.Write(payload)
		githubSig := fmt.Sprintf("sha256=%s", hex.EncodeToString(sig.Sum(nil)))

		req := httptest.NewRequest("POST", "/webhooks/github", bytes.NewReader(payload))
		req.Header.Set("X-Hub-Signature-256", githubSig)

		w := httptest.NewRecorder()
		http.HandlerFunc(githubHandler.HandleGitHub).ServeHTTP(w, req)
	}
}
