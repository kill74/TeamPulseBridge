package queue_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
	"teampulsebridge/services/ingestion-gateway/internal/testhelpers/pubsubtest"
)

// TestPubSubPublisherIntegration tests the PubSubPublisher against the Pub/Sub emulator.
// Requires PUBSUB_EMULATOR_HOST to be set.
func TestPubSubPublisherIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("skipping integration test: PUBSUB_EMULATOR_HOST is not set")
	}

	cfg := pubsubtest.DefaultEmulatorConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create client
	client, err := pubsubtest.NewPubSubClient(ctx, cfg)
	require.NoError(t, err, "failed to create Pub/Sub client")
	t.Cleanup(func() {
		assert.NoError(t, client.Close())
	})

	// Create topic for testing
	topic, err := pubsubtest.CreateTopic(ctx, client, "test-webhook-topic")
	require.NoError(t, err, "failed to create topic")

	// Create subscription to verify messages
	sub, err := pubsubtest.CreateSubscription(ctx, client, "test-sub", "test-webhook-topic")
	require.NoError(t, err, "failed to create subscription")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create publisher (without async wrapper for testing synchronous behavior)
	publisher := queue.NewPubSubPublisherDirect(client, topic, logger)

	t.Run("PublishMessage", func(t *testing.T) {
		// Arrange
		source := "slack"
		body := []byte(`{"type":"event_callback","event":{"type":"app_mention"}}`)
		headers := map[string]string{"X-Slack-Request-Timestamp": "1234567890"}

		// Act
		err := publisher.Publish(ctx, source, body, headers)

		// Assert
		require.NoError(t, err, "publish should succeed")

		// Verify message was published
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err, "should receive published message")
		assert.Len(t, messages, 1, "should receive exactly one message")

		// Verify message attributes
		assert.Equal(t, "slack", messages[0].Attributes["source"])
		assert.Equal(t, "raw-webhook-envelope", messages[0].Attributes["schema"])
		assert.Equal(t, "1", messages[0].Attributes["schema_version"])

		// Verify message body contains valid envelope
		var envelope struct {
			Source     string            `json:"source"`
			Headers    map[string]string `json:"headers"`
			Body       json.RawMessage   `json:"body"`
			ReceivedAt time.Time         `json:"received_at"`
		}
		err = json.Unmarshal(messages[0].Data, &envelope)
		assert.NoError(t, err, "message should contain valid JSON envelope")
		assert.Equal(t, "slack", envelope.Source)
		assert.Equal(t, headers, envelope.Headers)
		assert.Equal(t, json.RawMessage(body), envelope.Body)
	})

	t.Run("PublishMultipleMessages", func(t *testing.T) {
		// Purge subscription first
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err, "failed to purge subscription")

		// Arrange
		sources := []string{"github", "gitlab", "teams"}

		// Act & Assert
		for i, source := range sources {
			body := []byte(fmt.Sprintf(`{"event":{"id":"%d"}}`, i))
			headers := map[string]string{"X-Source": source}

			err := publisher.Publish(ctx, source, body, headers)
			require.NoError(t, err, "publish %s should succeed", source)
		}

		// Verify all messages were published
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 3, 5*time.Second)
		require.NoError(t, err, "should receive all messages")
		require.Len(t, messages, 3, "should receive exactly 3 messages")

		// Verify each message source
		sources_received := []string{}
		for _, msg := range messages {
			sources_received = append(sources_received, msg.Attributes["source"])
		}
		assert.ElementsMatch(t, sources, sources_received, "should receive all sources")
	})

	t.Run("PublishWithLargePayload", func(t *testing.T) {
		// Purge subscription first
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err, "failed to purge subscription")

		// Arrange - create a large JSON payload
		largePayload := struct {
			Events []map[string]interface{} `json:"events"`
		}{
			Events: make([]map[string]interface{}, 100),
		}
		for i := 0; i < 100; i++ {
			largePayload.Events[i] = map[string]interface{}{
				"id":   i,
				"data": fmt.Sprintf("This is event number %d", i),
			}
		}
		body, err := json.Marshal(largePayload)
		require.NoError(t, err, "should marshal large payload")

		// Act
		err = publisher.Publish(ctx, "github", body, map[string]string{})
		require.NoError(t, err, "publish large payload should succeed")

		// Assert
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err, "should receive published message")
		assert.Len(t, messages, 1, "should receive exactly one message")

		// Verify large payload was preserved
		var envelope struct {
			Body json.RawMessage `json:"body"`
		}
		err = json.Unmarshal(messages[0].Data, &envelope)
		assert.NoError(t, err, "should parse envelope")

		var receivedPayload struct {
			Events []interface{} `json:"events"`
		}
		err = json.Unmarshal(envelope.Body, &receivedPayload)
		assert.NoError(t, err, "should parse original payload")
		assert.Len(t, receivedPayload.Events, 100, "should preserve all events")
	})

	t.Run("PublishWithEmptyHeaders", func(t *testing.T) {
		// Purge subscription first
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err, "failed to purge subscription")

		// Arrange
		source := "slack"
		body := []byte(`{"type":"event"}`)
		headers := map[string]string{}

		// Act
		err = publisher.Publish(ctx, source, body, headers)
		require.NoError(t, err, "publish should succeed with empty headers")

		// Assert
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err, "should receive published message")
		assert.Len(t, messages, 1)

		var envelope struct {
			Headers map[string]string `json:"headers"`
		}
		err = json.Unmarshal(messages[0].Data, &envelope)
		assert.NoError(t, err, "should parse envelope")
		assert.NotNil(t, envelope.Headers, "headers should not be nil")
		assert.Len(t, envelope.Headers, 0, "headers should be empty")
	})

	t.Run("PublishConcurrentMessages", func(t *testing.T) {
		// Purge subscription first
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err, "failed to purge subscription")

		// Arrange
		messageCount := 10
		done := make(chan error, messageCount)

		// Act - publish concurrently
		for i := 0; i < messageCount; i++ {
			go func(index int) {
				body := []byte(fmt.Sprintf(`{"index":%d}`, index))
				err := publisher.Publish(ctx, "github", body, map[string]string{})
				done <- err
			}(i)
		}

		// Wait for all publishes
		for i := 0; i < messageCount; i++ {
			err := <-done
			require.NoError(t, err, "concurrent publish %d should succeed", i)
		}

		// Assert - verify all messages received
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, messageCount, 5*time.Second)
		require.NoError(t, err, "should receive all messages")
		assert.Len(t, messages, messageCount, "should receive all %d messages", messageCount)
	})

	t.Run("PublishMessageAttributes", func(t *testing.T) {
		// Purge subscription first
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err, "failed to purge subscription")

		// Arrange
		source := "github"
		headers := map[string]string{
			"X-GitHub-Event":    "pull_request",
			"X-GitHub-Delivery": "12345-67890",
		}
		body := []byte(`{"action":"opened"}`)

		// Act
		err = publisher.Publish(ctx, source, body, headers)
		require.NoError(t, err, "publish should succeed")

		// Assert
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, 1, 3*time.Second)
		assert.NoError(t, err, "should receive message")
		assert.Len(t, messages, 1)

		// Verify message attributes
		assert.Equal(t, "github", messages[0].Attributes["source"], "source attribute")
		assert.Equal(t, "raw-webhook-envelope", messages[0].Attributes["schema"], "schema attribute")
		assert.Equal(t, "1", messages[0].Attributes["schema_version"], "schema_version attribute")

		// Verify headers are in message body
		var envelope struct {
			Headers map[string]string `json:"headers"`
		}
		err = json.Unmarshal(messages[0].Data, &envelope)
		assert.NoError(t, err, "should parse envelope")
		assert.Equal(t, headers, envelope.Headers, "headers should match")
	})
}

// TestAsyncPublisherWithPubSub tests the AsyncPublisher wrapper around PubSubPublisher.
func TestAsyncPublisherWithPubSub(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("skipping integration test: PUBSUB_EMULATOR_HOST is not set")
	}

	cfg := pubsubtest.DefaultEmulatorConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create client
	client, err := pubsubtest.NewPubSubClient(ctx, cfg)
	require.NoError(t, err, "failed to create Pub/Sub client")
	t.Cleanup(func() {
		assert.NoError(t, client.Close())
	})

	// Create topic
	topic, err := pubsubtest.CreateTopic(ctx, client, "async-test-topic")
	require.NoError(t, err, "failed to create topic")

	// Create subscription
	sub, err := pubsubtest.CreateSubscription(ctx, client, "async-test-sub", "async-test-topic")
	require.NoError(t, err, "failed to create subscription")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create underlying publisher and wrap with async
	innerPublisher := queue.NewPubSubPublisherDirect(client, topic, logger)
	asyncPublisher := queue.NewAsyncPublisher(innerPublisher, 100, logger)

	t.Run("AsyncPublishMultipleMessages", func(t *testing.T) {
		// Arrange
		messageCount := 25

		// Act - publish messages without waiting for confirmation
		for i := 0; i < messageCount; i++ {
			body := []byte(fmt.Sprintf(`{"id":%d}`, i))
			// Async publish should not block
			err := asyncPublisher.Publish(ctx, "github", body, map[string]string{})
			require.NoError(t, err, "async publish should succeed")
		}

		// Give async publisher time to flush
		time.Sleep(500 * time.Millisecond)

		// Assert - verify all messages eventually appeared
		messages, err := pubsubtest.ReceiveMessages(ctx, sub, messageCount, 5*time.Second)
		assert.NoError(t, err, "should receive all async messages")
		assert.Len(t, messages, messageCount, "should receive all %d messages", messageCount)
	})

	t.Run("AsyncPublisherQueueFull", func(t *testing.T) {
		// Purge subscription first
		err := pubsubtest.PurgeSubscription(ctx, sub)
		require.NoError(t, err, "failed to purge subscription")

		// Create publisher with very small buffer
		smallBufferPublisher := queue.NewAsyncPublisher(innerPublisher, 5, logger)

		// Act - fill buffer and try to overflow
		var lastErr error
		for i := 0; i < 10; i++ {
			body := []byte(fmt.Sprintf(`{"id":%d}`, i))
			err := smallBufferPublisher.Publish(ctx, "github", body, map[string]string{})
			if err != nil {
				lastErr = err
				break
			}
		}

		// Assert - should eventually get queue full error
		assert.Error(t, lastErr, "should get error when queue is full")
		assert.ErrorIs(t, lastErr, queue.ErrQueueFull, "should be ErrQueueFull")
	})
}

// TestPubSubPublisherWithoutTopic tests publisher failures when topic doesn't exist.
func TestPubSubPublisherWithoutTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("PUBSUB_EMULATOR_HOST") == "" {
		t.Skip("skipping integration test: PUBSUB_EMULATOR_HOST is not set")
	}

	cfg := pubsubtest.DefaultEmulatorConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create client
	client, err := pubsubtest.NewPubSubClient(ctx, cfg)
	require.NoError(t, err, "failed to create Pub/Sub client")
	t.Cleanup(func() {
		assert.NoError(t, client.Close())
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Try to publish to non-existent topic
	nonExistentTopic := client.Publisher("this-topic-does-not-exist")
	publisher := queue.NewPubSubPublisherDirect(client, nonExistentTopic, logger)

	// Act & Assert
	err = publisher.Publish(ctx, "github", []byte(`{}`), map[string]string{})
	assert.Error(t, err, "publish to non-existent topic should fail")
}
