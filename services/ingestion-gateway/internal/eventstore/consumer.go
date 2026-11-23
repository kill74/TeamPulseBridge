// Package eventstore consumes raw webhook queue messages into durable storage.
package eventstore

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub/v2"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

// Store persists decoded webhook queue envelopes.
type Store interface {
	Save(ctx context.Context, in SaveInput) (Event, error)
}

// BatchStore optionally supports micro-batched writes. PostgresStore
// implements it via pgx Batch/CopyFrom; consumers prefer it when available.
type BatchStore interface {
	Store
	SaveBatch(ctx context.Context, inputs []SaveInput) ([]Event, error)
}

// Consumer receives raw webhook envelopes from Pub/Sub and writes them to a Store.
type Consumer struct {
	subscriber *pubsub.Subscriber
	store      Store
	logger     *slog.Logger
	// deliveryAttempts tracks poison-pill redeliveries for backoff.
	nacked     atomic.Int64
	stored     atomic.Int64
	maxRetries int
}

const defaultMaxDeliveryAttempts = 5

// NewConsumer creates a Pub/Sub event-store consumer.
func NewConsumer(subscriber *pubsub.Subscriber, store Store, logger *slog.Logger) *Consumer {
	return NewConsumerWithMaxRetries(subscriber, store, logger, defaultMaxDeliveryAttempts)
}

func NewConsumerWithMaxRetries(subscriber *pubsub.Subscriber, store Store, logger *slog.Logger, maxRetries int) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	if maxRetries <= 0 {
		maxRetries = defaultMaxDeliveryAttempts
	}
	return &Consumer{
		subscriber: subscriber,
		store:      store,
		logger:     logger,
		maxRetries: maxRetries,
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

	// Poison-pill guard: after N delivery attempts, Ack + DLQ-log instead of
	// tight Nack redelivery that saturates MaxOutstandingMessages.
	if msg.DeliveryAttempt != nil && *msg.DeliveryAttempt > c.maxRetries {
		c.nacked.Add(1)
		c.logger.Error("dropping poison-pill webhook event after max deliveries",
			"message_id", msg.ID,
			"source", envelope.Source,
			"delivery_attempt", *msg.DeliveryAttempt,
		)
		msg.Ack()
		return
	}

	event, err := c.store.Save(ctx, SaveInput{
		MessageID:       msg.ID,
		Envelope:        envelope,
		PublishedAt:     msg.PublishTime,
		DeliveryAttempt: msg.DeliveryAttempt,
		BodyHashHint:    msg.Attributes["body_hash"],
		Attrs:           msg.Attributes,
	})
	if err != nil {
		c.nacked.Add(1)
		// Nack immediately without sleeping: sleeping here blocks the Pub/Sub
		// receive callback goroutine (head-of-line blocking + lease expiry).
		// Redelivery backoff is handled by the subscription retry policy.
		backoff := 100 * time.Millisecond
		if msg.DeliveryAttempt != nil && *msg.DeliveryAttempt > 1 {
			backoff = time.Duration(*msg.DeliveryAttempt) * 200 * time.Millisecond
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
		c.logger.Error("failed to store webhook event", "message_id", msg.ID, "source", envelope.Source, "error", err, "backoff_ms", backoff.Milliseconds())
		msg.Nack()
		return
	}

	n := c.stored.Add(1)
	// Sampled logging: Debug every event, Info every 100th to avoid log I/O bottleneck.
	c.logger.Debug("stored webhook event", "message_id", msg.ID, "event_id", event.ID, "source", event.Source)
	if n%100 == 0 {
		c.logger.Info("event-store progress", "stored_total", n, "nacked_total", c.nacked.Load())
	}
	msg.Ack()
}
