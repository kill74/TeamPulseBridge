package eventstore

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

func TestConsumerAcksPoisonPillAfterMaxDeliveries(t *testing.T) {
	store := &fakeStore{}
	consumer := NewConsumer(nil, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	envelope := queue.NewRawWebhookEnvelope("github", []byte(`{"action":"opened"}`), map[string]string{}, time.Now())
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	attempt := defaultMaxDeliveryAttempts + 10

	// Must return without blocking: Nack-with-sleep here used to hold the
	// Pub/Sub callback goroutine (head-of-line blocking + lease expiry).
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumer.handleMessage(context.Background(), &pubsub.Message{
			ID:              "poison-1",
			Data:            payload,
			PublishTime:     envelope.ReceivedAt.Add(time.Second),
			DeliveryAttempt: &attempt,
		})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleMessage blocked on poison-pill; Nack path must return immediately")
	}

	if len(store.inputs) != 0 {
		t.Fatalf("expected no store calls for poison-pill, got %d", len(store.inputs))
	}
}
