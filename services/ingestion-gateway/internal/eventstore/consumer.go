// Package eventstore consumes raw webhook queue messages into durable storage.
package eventstore

import (
	"context"
	"fmt"
	"log/slog"

	"cloud.google.com/go/pubsub/v2"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

// Store persists decoded webhook queue envelopes.
type Store interface {
	Save(ctx context.Context, in SaveInput) (Event, error)
}

// Consumer receives raw webhook envelopes from Pub/Sub and writes them to a Store.
type Consumer struct {
	subscriber *pubsub.Subscriber
	store      Store
	logger     *slog.Logger
}

// NewConsumer creates a Pub/Sub event-store consumer.
func NewConsumer(subscriber *pubsub.Subscriber, store Store, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Consumer{
		subscriber: subscriber,
		store:      store,
		logger:     logger,
	}
}

// Run blocks while receiving Pub/Sub messages until the context is cancelled or receive fails.
func (c *Consumer) Run(ctx context.Context) error {
	if err := c.subscriber.Receive(ctx, c.handleMessage); err != nil {
		return fmt.Errorf("receive pubsub messages: %w", err)
	}
	return nil
}

func (c *Consumer) handleMessage(ctx context.Context, msg *pubsub.Message) {
	envelope, err := queue.DecodeRawWebhookEnvelope(msg.Data)
	if err != nil {
		c.logger.Warn("dropping invalid webhook envelope", "message_id", msg.ID, "error", err)
		msg.Ack()
		return
	}

	event, err := c.store.Save(ctx, SaveInput{
		MessageID:       msg.ID,
		Envelope:        envelope,
		PublishedAt:     msg.PublishTime,
		DeliveryAttempt: msg.DeliveryAttempt,
	})
	if err != nil {
		c.logger.Error("failed to store webhook event", "message_id", msg.ID, "source", envelope.Source, "error", err)
		msg.Nack()
		return
	}

	c.logger.Info("stored webhook event", "message_id", msg.ID, "event_id", event.ID, "source", event.Source)
	msg.Ack()
}
