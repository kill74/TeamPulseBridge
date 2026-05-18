package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

type fakeStore struct {
	inputs []SaveInput
	err    error
}

func (s *fakeStore) Save(_ context.Context, in SaveInput) (Event, error) {
	s.inputs = append(s.inputs, in)
	if s.err != nil {
		return Event{}, s.err
	}
	return Event{ID: 42, MessageID: in.MessageID, Source: in.Envelope.Source}, nil
}

func TestConsumerStoresValidEnvelope(t *testing.T) {
	store := &fakeStore{}
	consumer := NewConsumer(nil, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	envelope := queue.NewRawWebhookEnvelope("gitlab", []byte(`{"object_kind":"merge_request"}`), map[string]string{}, time.Now())
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	attempt := 1

	consumer.handleMessage(context.Background(), &pubsub.Message{
		ID:              "msg-1",
		Data:            payload,
		PublishTime:     envelope.ReceivedAt.Add(time.Second),
		DeliveryAttempt: &attempt,
	})

	if len(store.inputs) != 1 {
		t.Fatalf("expected one stored event, got %d", len(store.inputs))
	}
	if store.inputs[0].MessageID != "msg-1" {
		t.Fatalf("unexpected message id %q", store.inputs[0].MessageID)
	}
	if store.inputs[0].Envelope.Source != "gitlab" {
		t.Fatalf("unexpected source %q", store.inputs[0].Envelope.Source)
	}
	if store.inputs[0].DeliveryAttempt == nil || *store.inputs[0].DeliveryAttempt != 1 {
		t.Fatalf("unexpected delivery attempt %v", store.inputs[0].DeliveryAttempt)
	}
}

func TestConsumerDropsInvalidEnvelopeWithoutStoreCall(t *testing.T) {
	store := &fakeStore{}
	consumer := NewConsumer(nil, store, slog.New(slog.NewTextHandler(io.Discard, nil)))

	consumer.handleMessage(context.Background(), &pubsub.Message{
		ID:   "poison",
		Data: []byte(`{"source":"","headers":{},"body":{},"received_at":"2026-05-18T12:00:00Z","schema":"raw-webhook-envelope","schema_value":1}`),
	})

	if len(store.inputs) != 0 {
		t.Fatalf("expected no store calls, got %d", len(store.inputs))
	}
}

func TestConsumerNacksStoreFailureAfterDecode(t *testing.T) {
	store := &fakeStore{err: errors.New("postgres unavailable")}
	consumer := NewConsumer(nil, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	envelope := queue.NewRawWebhookEnvelope("teams", []byte(`{"value":[]}`), map[string]string{}, time.Now())
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	consumer.handleMessage(context.Background(), &pubsub.Message{ID: "msg-2", Data: payload})

	if len(store.inputs) != 1 {
		t.Fatalf("expected store call before nack, got %d", len(store.inputs))
	}
}
