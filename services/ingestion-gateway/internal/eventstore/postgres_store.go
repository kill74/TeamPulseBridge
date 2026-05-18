package eventstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore persists webhook events into Postgres.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore initializes the webhook event table and returns a Postgres-backed store.
func NewPostgresStore(ctx context.Context, pool *pgxpool.Pool) (*PostgresStore, error) {
	query := `
		CREATE TABLE IF NOT EXISTS webhook_events (
			id BIGSERIAL PRIMARY KEY,
			message_id TEXT NOT NULL UNIQUE,
			source TEXT NOT NULL,
			provider_event_id TEXT,
			schema TEXT NOT NULL,
			schema_value INT NOT NULL,
			received_at TIMESTAMP WITH TIME ZONE NOT NULL,
			published_at TIMESTAMP WITH TIME ZONE,
			stored_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			delivery_attempt INT,
			headers JSONB NOT NULL DEFAULT '{}',
			body JSONB NOT NULL,
			body_hash CHAR(64) NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_webhook_events_received_at ON webhook_events(received_at DESC);
		CREATE INDEX IF NOT EXISTS idx_webhook_events_source_received_at ON webhook_events(source, received_at DESC);
		CREATE INDEX IF NOT EXISTS idx_webhook_events_provider_event_id ON webhook_events(provider_event_id) WHERE provider_event_id IS NOT NULL;
		CREATE INDEX IF NOT EXISTS idx_webhook_events_body_hash ON webhook_events(body_hash);
	`
	if _, err := pool.Exec(ctx, query); err != nil {
		return nil, fmt.Errorf("create webhook_events table: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

// Save inserts a webhook event idempotently by Pub/Sub message ID.
func (s *PostgresStore) Save(ctx context.Context, in SaveInput) (Event, error) {
	record, err := BuildRecord(in)
	if err != nil {
		return Event{}, err
	}
	var publishedAt any
	if !record.PublishedAt.IsZero() {
		publishedAt = record.PublishedAt
	}
	var deliveryAttempt any
	if record.DeliveryAttempt != nil {
		deliveryAttempt = *record.DeliveryAttempt
	}

	var event Event
	query := `
		INSERT INTO webhook_events (
			message_id,
			source,
			provider_event_id,
			schema,
			schema_value,
			received_at,
			published_at,
			delivery_attempt,
			headers,
			body,
			body_hash
		)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (message_id) DO UPDATE SET message_id = EXCLUDED.message_id
		RETURNING id, message_id, source, COALESCE(provider_event_id, ''), schema, schema_value, received_at, COALESCE(published_at, '0001-01-01 00:00:00+00'::timestamptz), stored_at, body_hash
	`
	err = s.pool.QueryRow(ctx, query,
		record.MessageID,
		record.Source,
		record.ProviderEventID,
		record.Schema,
		record.SchemaValue,
		record.ReceivedAt,
		publishedAt,
		deliveryAttempt,
		record.HeadersJSON,
		record.BodyJSON,
		record.BodyHash,
	).Scan(
		&event.ID,
		&event.MessageID,
		&event.Source,
		&event.ProviderEventID,
		&event.Schema,
		&event.SchemaValue,
		&event.ReceivedAt,
		&event.PublishedAt,
		&event.StoredAt,
		&event.BodyHash,
	)
	if err != nil {
		return Event{}, fmt.Errorf("save webhook event: %w", err)
	}
	return event, nil
}
