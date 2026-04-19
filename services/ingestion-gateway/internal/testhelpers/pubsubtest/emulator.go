package pubsubtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// EmulatorConfig holds Pub/Sub emulator connection details.
type EmulatorConfig struct {
	ProjectID string
	HostPort  string
}

// DefaultEmulatorConfig returns standard test configuration.
func DefaultEmulatorConfig() EmulatorConfig {
	return EmulatorConfig{
		ProjectID: "test-project",
		HostPort:  "localhost:8085",
	}
}

// NewPubSubClient creates a client connected to the Pub/Sub emulator.
// It sets the PUBSUB_EMULATOR_HOST environment variable as required by the GCP SDK.
func NewPubSubClient(ctx context.Context, cfg EmulatorConfig) (*pubsub.Client, error) {
	// Set emulator host for SDK
	if err := os.Setenv("PUBSUB_EMULATOR_HOST", cfg.HostPort); err != nil {
		return nil, fmt.Errorf("failed to set PUBSUB_EMULATOR_HOST: %w", err)
	}

	client, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Pub/Sub client: %w", err)
	}

	return client, nil
}

// CreateTopic creates a topic in the emulator, creating it if it doesn't exist.
func CreateTopic(ctx context.Context, client *pubsub.Client, topicID string) (*pubsub.Publisher, error) {
	topicName := fmt.Sprintf("projects/%s/topics/%s", client.Project(), topicID)
	_, err := client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return nil, fmt.Errorf("failed to create topic %q: %w", topicID, err)
	}

	return client.Publisher(topicID), nil
}

// CreateSubscription creates a subscription to consume messages.
func CreateSubscription(ctx context.Context, client *pubsub.Client, subID, topicID string) (*pubsub.Subscriber, error) {
	subName := fmt.Sprintf("projects/%s/subscriptions/%s", client.Project(), subID)
	topicName := fmt.Sprintf("projects/%s/topics/%s", client.Project(), topicID)
	_, err := client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:               subName,
		Topic:              topicName,
		AckDeadlineSeconds: 30,
	})
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return nil, fmt.Errorf("failed to create subscription %q: %w", subID, err)
	}

	return client.Subscriber(subID), nil
}

// ReceiveMessages receives up to count messages from a subscription.
// It returns immediately if count messages are received before timeout.
func ReceiveMessages(ctx context.Context, sub *pubsub.Subscriber, count int, timeout time.Duration) ([]*pubsub.Message, error) {
	if count <= 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Make callback execution deterministic and avoid concurrent appends during tests.
	prevNumGoroutines := sub.ReceiveSettings.NumGoroutines
	prevMaxOutstandingMessages := sub.ReceiveSettings.MaxOutstandingMessages
	sub.ReceiveSettings.NumGoroutines = 1
	sub.ReceiveSettings.MaxOutstandingMessages = 1
	defer func() {
		sub.ReceiveSettings.NumGoroutines = prevNumGoroutines
		sub.ReceiveSettings.MaxOutstandingMessages = prevMaxOutstandingMessages
	}()

	messages := make([]*pubsub.Message, 0, count)
	var mu sync.Mutex
	err := sub.Receive(ctx, func(_ context.Context, msg *pubsub.Message) {
		mu.Lock()
		if len(messages) < count {
			messages = append(messages, msg)
		}
		reachedTarget := len(messages) >= count
		mu.Unlock()

		msg.Ack()

		// Stop receiving if we have enough messages
		if reachedTarget {
			cancel()
		}
	})

	mu.Lock()
	collected := append([]*pubsub.Message(nil), messages...)
	mu.Unlock()

	// Context cancel is not an error when we got our messages
	if err == context.Canceled && len(collected) >= count {
		return collected, nil
	}

	if err != nil {
		return collected, fmt.Errorf("failed to receive messages: %w", err)
	}

	if len(collected) < count {
		return collected, fmt.Errorf("received %d/%d messages before timeout", len(collected), count)
	}

	return collected, nil
}

// PurgeSubscription deletes all messages in a subscription.
// This is useful for cleaning up between tests.
func PurgeSubscription(ctx context.Context, sub *pubsub.Subscriber) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Receive and ack all messages (discarding them)
	err := sub.Receive(ctx, func(_ context.Context, msg *pubsub.Message) {
		msg.Ack()
	})
	if err == context.Canceled || err == context.DeadlineExceeded {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to purge subscription: %w", err)
	}
	return nil
}

// DeleteTopic deletes a topic.
func DeleteTopic(ctx context.Context, client *pubsub.Client, topicID string) error {
	topicName := fmt.Sprintf("projects/%s/topics/%s", client.Project(), topicID)
	err := client.TopicAdminClient.DeleteTopic(ctx, &pubsubpb.DeleteTopicRequest{Topic: topicName})
	if status.Code(err) == codes.NotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete topic %q: %w", topicID, err)
	}

	return nil
}

// DeleteSubscription deletes a subscription.
func DeleteSubscription(ctx context.Context, client *pubsub.Client, subID string) error {
	subName := fmt.Sprintf("projects/%s/subscriptions/%s", client.Project(), subID)
	err := client.SubscriptionAdminClient.DeleteSubscription(ctx, &pubsubpb.DeleteSubscriptionRequest{
		Subscription: subName,
	})
	if status.Code(err) == codes.NotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to delete subscription %q: %w", subID, err)
	}

	return nil
}

// MessageCollector collects messages from a subscription for easier testing.
type MessageCollector struct {
	mu       sync.Mutex
	messages []*pubsub.Message
	done     chan struct{}
}

// NewMessageCollector creates a new message collector.
func NewMessageCollector() *MessageCollector {
	return &MessageCollector{
		messages: make([]*pubsub.Message, 0),
		done:     make(chan struct{}),
	}
}

// Collect starts collecting messages from a subscription in the background.
// Call Stop() to stop collection.
func (mc *MessageCollector) Collect(ctx context.Context, sub *pubsub.Subscriber) {
	go func() {
		_ = sub.Receive(ctx, func(_ context.Context, msg *pubsub.Message) {
			mc.mu.Lock()
			mc.messages = append(mc.messages, msg)
			mc.mu.Unlock()
			msg.Ack()
		})
		close(mc.done)
	}()
}

// Stop stops collection and returns all collected messages.
func (mc *MessageCollector) Stop(ctx context.Context, timeout time.Duration) []*pubsub.Message {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-mc.done:
	case <-ctx.Done():
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()
	return append([]*pubsub.Message(nil), mc.messages...)
}

// Messages returns all collected messages so far (without stopping collection).
func (mc *MessageCollector) Messages() []*pubsub.Message {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return append([]*pubsub.Message(nil), mc.messages...)
}

// Count returns the number of collected messages.
func (mc *MessageCollector) Count() int {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return len(mc.messages)
}

// FindMessage finds the first message matching a predicate.
func (mc *MessageCollector) FindMessage(predicate func(*pubsub.Message) bool) *pubsub.Message {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	for _, msg := range mc.messages {
		if predicate(msg) {
			return msg
		}
	}
	return nil
}
