package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
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
	transport      *http.Transport
	closeOnce      sync.Once
	healthMu       sync.Mutex
	lastHealthAt   time.Time
	lastHealthErr  error
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
		} else {
			// Sensible default: 16 parallel gRPC streams so a single
			// AsyncPublisher worker pool doesn't serialize on 1 stream.
			p.topic.PublishSettings.NumGoroutines = 16
		}
	}
}

func WithPublishFlowControl(maxOutstandingMessages, maxOutstandingBytes int, behavior string) PubSubOption {
	return func(p *PubSubPublisher) {
		if maxOutstandingMessages > 0 {
			p.topic.PublishSettings.FlowControlSettings.MaxOutstandingMessages = maxOutstandingMessages
		} else {
			p.topic.PublishSettings.FlowControlSettings.MaxOutstandingMessages = 2000
		}
		if maxOutstandingBytes > 0 {
			p.topic.PublishSettings.FlowControlSettings.MaxOutstandingBytes = maxOutstandingBytes
		} else {
			p.topic.PublishSettings.FlowControlSettings.MaxOutstandingBytes = 100 << 20
		}
		if behavior == "" {
			behavior = "signal_error"
		}
		p.topic.PublishSettings.FlowControlSettings.LimitExceededBehavior = pubsubLimitExceededBehavior(behavior)
	}
}

// WithPublishBatching tunes Pub/Sub server-side bundling. Small delay (10-50ms)
// dramatically raises throughput by packing many webhook envelopes per RPC.
func WithPublishBatching(delay time.Duration, countThreshold, byteThreshold int) PubSubOption {
	return func(p *PubSubPublisher) {
		if delay > 0 {
			p.topic.PublishSettings.DelayThreshold = delay
		}
		if countThreshold > 0 {
			p.topic.PublishSettings.CountThreshold = countThreshold
		}
		if byteThreshold > 0 {
			p.topic.PublishSettings.ByteThreshold = byteThreshold
		}
	}
}

func NewPubSubPublisher(ctx context.Context, projectID, topicID string, logger *slog.Logger, opts ...PubSubOption) (*PubSubPublisher, error) {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	client, err := pubsub.NewClient(ctx, projectID, option.WithHTTPClient(httpClient))
	if err != nil {
		transport.CloseIdleConnections()
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
	// Production-grade bundling defaults: 20ms delay, 500 msgs, 1MiB.
	// Overridden by WithPublishBatching when config provides values.
	if topic.PublishSettings.DelayThreshold == 0 {
		topic.PublishSettings.DelayThreshold = 20 * time.Millisecond
	}
	if topic.PublishSettings.CountThreshold == 0 {
		topic.PublishSettings.CountThreshold = 500
	}
	if topic.PublishSettings.ByteThreshold == 0 {
		topic.PublishSettings.ByteThreshold = 1 << 20
	}
	if topic.PublishSettings.NumGoroutines <= 0 {
		topic.PublishSettings.NumGoroutines = 16
	}
	if topic.PublishSettings.FlowControlSettings.MaxOutstandingMessages == 0 {
		topic.PublishSettings.FlowControlSettings.MaxOutstandingMessages = 2000
	}
	if topic.PublishSettings.FlowControlSettings.MaxOutstandingBytes == 0 {
		topic.PublishSettings.FlowControlSettings.MaxOutstandingBytes = 100 << 20
	}
	p := &PubSubPublisher{
		client:         client,
		transport:      transport,
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
	if len(body) > 1<<20 {
		return fmt.Errorf("pubsub payload exceeds 1MiB: %d bytes", len(body))
	}
	// No-copy: async workers already own their clone, so skip the 3rd copy.
	// Headers are not mutated after this point (marshal reads only).
	envelope := NewRawWebhookEnvelopeNoCopy(source, body, headers, time.Now())
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal pubsub payload: %w", err)
	}

	msgAttrs := map[string]string{
		"source":         source,
		"schema":         envelope.Schema,
		"schema_version": strconv.Itoa(envelope.SchemaValue),
		"body_len":       strconv.Itoa(len(envelope.Body)),
	}
	if traceparent := headers["Traceparent"]; traceparent != "" {
		msgAttrs["traceparent"] = traceparent
	}
	if requestID := headers["X-Request-Id"]; requestID != "" {
		msgAttrs["x-request-id"] = requestID
	}
	if eid := headers["X-Event-ID"]; eid != "" {
		msgAttrs["event_id"] = eid
	}
	// body_hash hint for the consumer (computed in the worker, off the hot path).
	// Only for ≤256KiB to avoid hashing 1MiB twice; larger bodies hash on consume.
	if len(envelope.Body) > 0 && len(envelope.Body) <= 262144 {
		sum := sha256.Sum256(envelope.Body)
		msgAttrs["body_hash"] = hex.EncodeToString(sum[:])
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
		// Floor stale async deadlines to 1s so near-expired requests don't
		// instantly fail with DeadlineExceeded and burn DLQ.
		if remaining < time.Second {
			remaining = time.Second
		}
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
		if err := p.client.Close(); err != nil {
			closeErr = fmt.Errorf("close pubsub client: %w", err)
		}
		if p.transport != nil {
			p.transport.CloseIdleConnections()
		}
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
	// Cache successful checks for 10s to avoid an admin RPC storm on scrapes.
	p.healthMu.Lock()
	sinceCheck := time.Since(p.lastHealthAt)
	cachedErr := p.lastHealthErr
	wasChecked := !p.lastHealthAt.IsZero()
	p.healthMu.Unlock()
	if wasChecked {
		if cachedErr == nil && sinceCheck < 10*time.Second {
			return nil
		}
		if cachedErr != nil && sinceCheck < 2*time.Second {
			return cachedErr
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := p.client.TopicAdminClient.GetTopic(checkCtx, &pubsubpb.GetTopicRequest{Topic: p.topicName})
	p.healthMu.Lock()
	p.lastHealthAt = time.Now()
	p.lastHealthErr = err
	p.healthMu.Unlock()
	if err != nil {
		return fmt.Errorf("pubsub health check failed: %w", err)
	}
	return nil
}
