package pubsubtest

import (
	"context"
	"fmt"
	"os"
	"time"

	"cloud.google.com/go/pubsub"
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
func CreateTopic(ctx context.Context, client *pubsub.Client, topicID string) (*pubsub.Topic, error) {
	topic := client.Topic(topicID)

	// Check if topic exists
	exists, err := topic.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check topic existence: %w", err)
	}

	if exists {
		return topic, nil
	}

	// Create topic if it doesn't exist
	topic, err = client.CreateTopic(ctx, topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to create topic %q: %w", topicID, err)
	}

	return topic, nil
}

// CreateSubscription creates a subscription to consume messages.
func CreateSubscription(ctx context.Context, client *pubsub.Client, subID, topicID string) (*pubsub.Subscription, error) {
	sub := client.Subscription(subID)

	// Check if subscription exists
	exists, err := sub.Exists(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check subscription existence: %w", err)
	}

	if exists {
		return sub, nil
	}

	// Create subscription if it doesn't exist
	topic := client.Topic(topicID)
	sub, err = client.CreateSubscription(ctx, subID, pubsub.SubscriptionConfig{
		Topic:       topic,
		AckDeadline: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription %q: %w", subID, err)
	}

	return sub, nil
}

// ReceiveMessages receives up to count messages from a subscription.
// It returns immediately if count messages are received before timeout.
func ReceiveMessages(ctx context.Context, sub *pubsub.Subscription, count int, timeout time.Duration) ([]*pubsub.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var messages []*pubsub.Message
	err := sub.Receive(ctx, func(_ context.Context, msg *pubsub.Message) {
		messages = append(messages, msg)
		msg.Ack()

		// Stop receiving if we have enough messages
		if len(messages) >= count {
			cancel()
		}
	})

	// Context cancel is not an error when we got our messages
	if err == context.Canceled && len(messages) >= count {
		return messages, nil
	}

	if err != nil {
		return messages, fmt.Errorf("failed to receive messages: %w", err)
	}

	return messages, nil
}

// PurgeSubscription deletes all messages in a subscription.
// This is useful for cleaning up between tests.
func PurgeSubscription(ctx context.Context, sub *pubsub.Subscription) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Receive and ack all messages (discarding them)
	return sub.Receive(ctx, func(_ context.Context, msg *pubsub.Message) {
		msg.Ack()
	})
}

// DeleteTopic deletes a topic.
func DeleteTopic(ctx context.Context, client *pubsub.Client, topicID string) error {
	topic := client.Topic(topicID)
	exists, err := topic.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check topic existence: %w", err)
	}

	if !exists {
		return nil
	}

	if err := topic.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete topic %q: %w", topicID, err)
	}

	return nil
}

// DeleteSubscription deletes a subscription.
func DeleteSubscription(ctx context.Context, client *pubsub.Client, subID string) error {
	sub := client.Subscription(subID)
	exists, err := sub.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check subscription existence: %w", err)
	}

	if !exists {
		return nil
	}

	if err := sub.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete subscription %q: %w", subID, err)
	}

	return nil
}

// MessageCollector collects messages from a subscription for easier testing.
type MessageCollector struct {
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
func (mc *MessageCollector) Collect(ctx context.Context, sub *pubsub.Subscription) {
	go func() {
		sub.Receive(ctx, func(_ context.Context, msg *pubsub.Message) {
			mc.messages = append(mc.messages, msg)
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

	return mc.messages
}

// Messages returns all collected messages so far (without stopping collection).
func (mc *MessageCollector) Messages() []*pubsub.Message {
	return mc.messages
}

// Count returns the number of collected messages.
func (mc *MessageCollector) Count() int {
	return len(mc.messages)
}

// FindMessage finds the first message matching a predicate.
func (mc *MessageCollector) FindMessage(predicate func(*pubsub.Message) bool) *pubsub.Message {
	for _, msg := range mc.messages {
		if predicate(msg) {
			return msg
		}
	}
	return nil
}
