package queue

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewRawWebhookEnvelopeCopiesInputs(t *testing.T) {
	headers := map[string]string{"X-Request-Id": "req-123"}
	body := []byte(`{"action":"opened"}`)
	receivedAt := time.Date(2026, 5, 18, 12, 0, 0, 0, time.FixedZone("WEST", 3600))

	envelope := NewRawWebhookEnvelope("github", body, headers, receivedAt)
	headers["X-Request-Id"] = "mutated"
	body[0] = '['

	if envelope.Headers["X-Request-Id"] != "req-123" {
		t.Fatalf("expected headers to be copied, got %q", envelope.Headers["X-Request-Id"])
	}
	if string(envelope.Body) != `{"action":"opened"}` {
		t.Fatalf("expected body to be copied, got %s", envelope.Body)
	}
	if envelope.ReceivedAt.Location() != time.UTC {
		t.Fatalf("expected received_at to be stored in UTC, got %s", envelope.ReceivedAt.Location())
	}
	if envelope.Schema != RawWebhookEnvelopeSchema || envelope.SchemaValue != RawWebhookEnvelopeSchemaVersion {
		t.Fatalf("unexpected schema identity: %+v", envelope)
	}
}

func TestDecodeRawWebhookEnvelopeValidatesContract(t *testing.T) {
	payload, err := json.Marshal(NewRawWebhookEnvelope(
		"slack",
		[]byte(`{"type":"event_callback"}`),
		map[string]string{"Content-Type": "application/json"},
		time.Now(),
	))
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	envelope, err := DecodeRawWebhookEnvelope(payload)
	if err != nil {
		t.Fatalf("DecodeRawWebhookEnvelope() error = %v", err)
	}
	if envelope.Source != "slack" {
		t.Fatalf("unexpected source %q", envelope.Source)
	}
}

func TestDecodeRawWebhookEnvelopeRejectsInvalidBody(t *testing.T) {
	payload := []byte(`{
		"source":"github",
		"headers":{},
		"body":,
		"received_at":"2026-05-18T12:00:00Z",
		"schema":"raw-webhook-envelope",
		"schema_value":1
	}`)

	if _, err := DecodeRawWebhookEnvelope(payload); err == nil {
		t.Fatal("expected invalid envelope error")
	}
}
