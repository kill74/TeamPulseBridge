package eventstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

// SaveInput contains the Pub/Sub message identity and webhook envelope to store.
type SaveInput struct {
	MessageID       string
	Envelope        queue.RawWebhookEnvelope
	PublishedAt     time.Time
	DeliveryAttempt *int
}

// Event is the persisted representation of a queued webhook event.
type Event struct {
	ID              int64
	MessageID       string
	Source          string
	ProviderEventID string
	Schema          string
	SchemaValue     int
	ReceivedAt      time.Time
	PublishedAt     time.Time
	StoredAt        time.Time
	BodyHash        string
}

// Record is the normalized database write model derived from a queue envelope.
type Record struct {
	MessageID       string
	Source          string
	ProviderEventID string
	Schema          string
	SchemaValue     int
	ReceivedAt      time.Time
	PublishedAt     time.Time
	DeliveryAttempt *int
	HeadersJSON     []byte
	BodyJSON        []byte
	BodyHash        string
}

// BuildRecord validates and normalizes a queue envelope for durable storage.
func BuildRecord(in SaveInput) (Record, error) {
	messageID := strings.TrimSpace(in.MessageID)
	if messageID == "" {
		return Record{}, errors.New("message id is required")
	}
	if err := in.Envelope.Validate(); err != nil {
		return Record{}, fmt.Errorf("validate raw webhook envelope: %w", err)
	}

	headers := in.Envelope.Headers
	if headers == nil {
		headers = map[string]string{}
	}
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return Record{}, fmt.Errorf("marshal event headers: %w", err)
	}

	bodyJSON := append([]byte(nil), in.Envelope.Body...)
	sum := sha256.Sum256(bodyJSON)

	return Record{
		MessageID:       messageID,
		Source:          in.Envelope.Source,
		ProviderEventID: providerEventID(headers),
		Schema:          in.Envelope.Schema,
		SchemaValue:     in.Envelope.SchemaValue,
		ReceivedAt:      in.Envelope.ReceivedAt.UTC(),
		PublishedAt:     in.PublishedAt.UTC(),
		DeliveryAttempt: in.DeliveryAttempt,
		HeadersJSON:     headersJSON,
		BodyJSON:        bodyJSON,
		BodyHash:        hex.EncodeToString(sum[:]),
	}, nil
}

func providerEventID(headers map[string]string) string {
	candidates := []string{
		"X-GitHub-Delivery",
		"X-Gitlab-Event-UUID",
		"X-GitLab-Event-UUID",
		"X-Slack-Retry-Num",
		"X-Request-Id",
		"X-Request-ID",
	}
	for _, key := range candidates {
		if value := strings.TrimSpace(headers[key]); value != "" {
			return value
		}
	}
	return ""
}
