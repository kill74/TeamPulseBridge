package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/pubsub"
)

type PubSubPublisher struct {
	client *pubsub.Client
	topic  *pubsub.Topic
	logger *slog.Logger
}

type pubSubEnvelope struct {
	Source      string            `json:"source"`
	Headers     map[string]string `json:"headers"`
	Body        []byte            `json:"body"`
	ReceivedAt  time.Time         `json:"received_at"`
	Schema      string            `json:"schema"`
	SchemaValue int               `json:"schema_value"`
}

func NewPubSubPublisher(ctx context.Context, projectID, topicID string, logger *slog.Logger) (*PubSubPublisher, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}
	topic := client.Topic(topicID)
	exists, err := topic.Exists(ctx)
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("check topic existence: %w", err)
	}
	if !exists {
		_ = client.Close()
		return nil, fmt.Errorf("pubsub topic %q does not exist", topicID)
	}
	return &PubSubPublisher{client: client, topic: topic, logger: logger}, nil
}

func (p *PubSubPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	envelope := pubSubEnvelope{
		Source:      source,
		Headers:     headers,
		Body:        body,
		ReceivedAt:  time.Now().UTC(),
		Schema:      "raw-webhook-envelope",
		SchemaValue: 1,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal pubsub payload: %w", err)
	}

	msg := &pubsub.Message{
		Data: payload,
		Attributes: map[string]string{
			"source":         source,
			"schema":         envelope.Schema,
			"schema_version": "1",
		},
	}

	res := p.topic.Publish(ctx, msg)
	msgID, err := res.Get(ctx)
	if err != nil {
		return fmt.Errorf("pubsub publish: %w", err)
	}
	p.logger.Debug("pubsub publish ok", "source", source, "message_id", msgID)
	return nil
}

func (p *PubSubPublisher) Close() error {
	p.topic.Stop()
	return p.client.Close()
}
