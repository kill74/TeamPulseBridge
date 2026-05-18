package eventstore

import (
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

func TestBuildRecordComputesStableFields(t *testing.T) {
	receivedAt := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	publishedAt := receivedAt.Add(time.Second)
	attempt := 2
	envelope := queue.NewRawWebhookEnvelope(
		"github",
		[]byte(`{"action":"opened"}`),
		map[string]string{"X-GitHub-Delivery": "delivery-123"},
		receivedAt,
	)

	record, err := BuildRecord(SaveInput{
		MessageID:       "msg-1",
		Envelope:        envelope,
		PublishedAt:     publishedAt,
		DeliveryAttempt: &attempt,
	})
	if err != nil {
		t.Fatalf("BuildRecord() error = %v", err)
	}

	if record.MessageID != "msg-1" {
		t.Fatalf("unexpected message id %q", record.MessageID)
	}
	if record.ProviderEventID != "delivery-123" {
		t.Fatalf("unexpected provider event id %q", record.ProviderEventID)
	}
	if record.BodyHash != "d592421cfe150deec6c49b8989cc99478e39c7f8cdd4c36f5b1c4cfeff394e24" {
		t.Fatalf("unexpected body hash %q", record.BodyHash)
	}
	if string(record.HeadersJSON) != `{"X-GitHub-Delivery":"delivery-123"}` {
		t.Fatalf("unexpected headers json %s", record.HeadersJSON)
	}
	if string(record.BodyJSON) != `{"action":"opened"}` {
		t.Fatalf("unexpected body json %s", record.BodyJSON)
	}
	if record.DeliveryAttempt == nil || *record.DeliveryAttempt != 2 {
		t.Fatalf("unexpected delivery attempt %v", record.DeliveryAttempt)
	}
}

func TestBuildRecordRejectsMissingMessageID(t *testing.T) {
	envelope := queue.NewRawWebhookEnvelope("slack", []byte(`{"type":"event_callback"}`), map[string]string{}, time.Now())

	if _, err := BuildRecord(SaveInput{Envelope: envelope}); err == nil {
		t.Fatal("expected missing message id error")
	}
}

func TestProviderEventIDFallsBackToRequestID(t *testing.T) {
	got := providerEventID(map[string]string{"X-Request-Id": "req-123"})
	if got != "req-123" {
		t.Fatalf("providerEventID() = %q, want req-123", got)
	}
}
