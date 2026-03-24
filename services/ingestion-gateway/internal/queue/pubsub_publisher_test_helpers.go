package queue

import (
	"cloud.google.com/go/pubsub"
	"log/slog"
)

// NewPubSubPublisherDirect creates a PubSubPublisher directly with provided client and topic.
// This is primarily for testing and should not be used in production.
// In production, use NewPubSubPublisher factory function which validates the topic exists.
func NewPubSubPublisherDirect(client *pubsub.Client, topic *pubsub.Topic, logger *slog.Logger) *PubSubPublisher {
	return &PubSubPublisher{
		client: client,
		topic:  topic,
		logger: logger,
	}
}
