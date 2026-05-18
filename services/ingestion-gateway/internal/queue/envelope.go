// Package queue contains webhook event publisher implementations and queue contracts.
package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	// RawWebhookEnvelopeSchema is the schema name for durable raw webhook queue messages.
	RawWebhookEnvelopeSchema = "raw-webhook-envelope"
	// RawWebhookEnvelopeSchemaVersion is the current raw webhook envelope schema version.
	RawWebhookEnvelopeSchemaVersion = 1
)

// RawWebhookEnvelope is the durable queue contract emitted by webhook handlers.
type RawWebhookEnvelope struct {
	Source      string            `json:"source"`
	Headers     map[string]string `json:"headers"`
	Body        json.RawMessage   `json:"body"`
	ReceivedAt  time.Time         `json:"received_at"`
	Schema      string            `json:"schema"`
	SchemaValue int               `json:"schema_value"`
}

// pubSubEnvelope is kept as a package-local alias for existing tests and code
// that refer to the historical unexported name.
type pubSubEnvelope = RawWebhookEnvelope

// NewRawWebhookEnvelope builds a copy-safe queue envelope for a raw webhook payload.
func NewRawWebhookEnvelope(source string, body []byte, headers map[string]string, receivedAt time.Time) RawWebhookEnvelope {
	copiedHeaders := make(map[string]string, len(headers))
	for k, v := range headers {
		copiedHeaders[k] = v
	}
	copiedBody := append([]byte(nil), body...)
	return RawWebhookEnvelope{
		Source:      source,
		Headers:     copiedHeaders,
		Body:        json.RawMessage(copiedBody),
		ReceivedAt:  receivedAt.UTC(),
		Schema:      RawWebhookEnvelopeSchema,
		SchemaValue: RawWebhookEnvelopeSchemaVersion,
	}
}

// DecodeRawWebhookEnvelope unmarshals and validates a durable queue envelope.
func DecodeRawWebhookEnvelope(payload []byte) (RawWebhookEnvelope, error) {
	var envelope RawWebhookEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return RawWebhookEnvelope{}, fmt.Errorf("decode raw webhook envelope: %w", err)
	}
	if err := envelope.Validate(); err != nil {
		return RawWebhookEnvelope{}, err
	}
	return envelope, nil
}

// Validate checks whether the envelope satisfies the raw webhook queue contract.
func (e RawWebhookEnvelope) Validate() error {
	if e.Source == "" {
		return errors.New("raw webhook envelope source is required")
	}
	if e.Schema != RawWebhookEnvelopeSchema {
		return fmt.Errorf("raw webhook envelope schema must be %q, got %q", RawWebhookEnvelopeSchema, e.Schema)
	}
	if e.SchemaValue != RawWebhookEnvelopeSchemaVersion {
		return fmt.Errorf("raw webhook envelope schema_value must be %d, got %d", RawWebhookEnvelopeSchemaVersion, e.SchemaValue)
	}
	if e.ReceivedAt.IsZero() {
		return errors.New("raw webhook envelope received_at is required")
	}
	if len(e.Body) == 0 {
		return errors.New("raw webhook envelope body is required")
	}
	if !json.Valid(e.Body) {
		return errors.New("raw webhook envelope body must be valid JSON")
	}
	if e.Headers == nil {
		return errors.New("raw webhook envelope headers is required")
	}
	return nil
}
