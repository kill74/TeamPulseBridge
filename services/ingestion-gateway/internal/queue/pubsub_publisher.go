package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PubSubPublisher struct {
	client         *pubsub.Client
	topic          *pubsub.Publisher
	topicName      string
	logger         *slog.Logger
	publishTimeout time.Duration
	closeOnce      sync.Once
}

type pubSubEnvelope struct {
	Source      string            `json:"source"`
	Headers     map[string]string `json:"headers"`
	Body        json.RawMessage   `json:"body"`
	ReceivedAt  time.Time         `json:"received_at"`
	Schema      string            `json:"schema"`
	SchemaValue int               `json:"schema_value"`
}

type PubSubOption func(*PubSubPublisher)

func WithPublishTimeout(d time.Duration) PubSubOption {
	return func(p *PubSubPublisher) {
		if d > 0 {
			p.publishTimeout = d
		}
	}
}

func WithPublishGoroutines(n int) PubSubOption {
	return func(p *PubSubPublisher) {
		if n > 0 {
			p.topic.PublishSettings.NumGoroutines = n
		}
	}
}

func WithPublishFlowControl(maxOutstandingMessages, maxOutstandingBytes int, behavior string) PubSubOption {
	return func(p *PubSubPublisher) {
		if maxOutstandingMessages > 0 {
			p.topic.PublishSettings.FlowControlSettings.MaxOutstandingMessages = maxOutstandingMessages
		}
		if maxOutstandingBytes > 0 {
			p.topic.PublishSettings.FlowControlSettings.MaxOutstandingBytes = maxOutstandingBytes
		}
		p.topic.PublishSettings.FlowControlSettings.LimitExceededBehavior = pubsubLimitExceededBehavior(behavior)
	}
}

func NewPubSubPublisher(ctx context.Context, projectID, topicID string, logger *slog.Logger, opts ...PubSubOption) (*PubSubPublisher, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 50,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 30 * time.Second,
	}
	client, err := pubsub.NewClient(ctx, projectID, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}

	topicName := fmt.Sprintf("projects/%s/topics/%s", projectID, topicID)
	if _, err := client.TopicAdminClient.GetTopic(ctx, &pubsubpb.GetTopicRequest{Topic: topicName}); err != nil {
		if closeErr := client.Close(); closeErr != nil {
			return nil, fmt.Errorf("check topic existence: %w; client close error: %w", err, closeErr)
		}
		if status.Code(err) == codes.NotFound {
			return nil, fmt.Errorf("pubsub topic %q does not exist", topicID)
		}
		return nil, fmt.Errorf("check topic existence: %w", err)
	}

	topic := client.Publisher(topicID)
	p := &PubSubPublisher{
		client:         client,
		topic:          topic,
		topicName:      topicName,
		logger:         logger,
		publishTimeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p, nil
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

	msgAttrs := map[string]string{
		"source":         source,
		"schema":         envelope.Schema,
		"schema_version": "1",
	}
	if traceparent := headers["Traceparent"]; traceparent != "" {
		msgAttrs["traceparent"] = traceparent
	}
	if requestID := headers["X-Request-Id"]; requestID != "" {
		msgAttrs["x-request-id"] = requestID
	}

	msg := &pubsub.Message{
		Data:       payload,
		Attributes: msgAttrs,
	}

	timeout := p.publishTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}
	publishCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res := p.topic.Publish(publishCtx, msg)
	msgID, err := res.Get(publishCtx)
	if err != nil {
		return fmt.Errorf("pubsub publish: %w", err)
	}
	p.logger.Debug("pubsub publish ok", "source", source, "message_id", msgID)
	return nil
}

func (p *PubSubPublisher) Close() error {
	var closeErr error
	p.closeOnce.Do(func() {
		p.topic.Stop()
		closeErr = p.client.Close()
	})
	return closeErr
}

func pubsubLimitExceededBehavior(behavior string) pubsub.LimitExceededBehavior {
	switch strings.TrimSpace(strings.ToLower(behavior)) {
	case "block":
		return pubsub.FlowControlBlock
	case "signal_error", "signal-error", "error":
		return pubsub.FlowControlSignalError
	default:
		return pubsub.FlowControlIgnore
	}
}

func (p *PubSubPublisher) HealthCheck(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := p.client.TopicAdminClient.GetTopic(checkCtx, &pubsubpb.GetTopicRequest{Topic: p.topicName})
	if err != nil {
		return fmt.Errorf("pubsub health check failed: %w", err)
	}
	return nil
}
