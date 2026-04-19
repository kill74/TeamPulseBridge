package queue

import (
	"log/slog"

	"cloud.google.com/go/pubsub/v2"
)

// NewPubSubPublisherDirect creates a PubSubPublisher directly with provided client and topic.
// This is primarily for testing and should not be used in production.
// In production, use NewPubSubPublisher factory function which validates the topic exists.
func NewPubSubPublisherDirect(client *pubsub.Client, topic *pubsub.Publisher, logger *slog.Logger) *PubSubPublisher {
	return &PubSubPublisher{
		client: client,
		topic:  topic,
		logger: logger,
	}
}
