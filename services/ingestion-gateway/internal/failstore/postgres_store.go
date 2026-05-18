package failstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		CREATE TABLE IF NOT EXISTS failed_events (
			event_id VARCHAR(64) PRIMARY KEY,
			source VARCHAR(255) NOT NULL,
			reason TEXT NOT NULL,
			payload_hash VARCHAR(64) NOT NULL,
			failed_at TIMESTAMP WITH TIME ZONE NOT NULL,
			retry_count INT NOT NULL DEFAULT 0,
			headers JSONB NOT NULL DEFAULT '{}',
			body BYTEA NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_failed_events_failed_at ON failed_events(failed_at DESC);
	`
	if _, err := pool.Exec(ctx, query); err != nil {
		panic(fmt.Sprintf("failed to create failstore tables: %v", err))
	}

	return &PostgresStore{
		pool: pool,
	}
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *PostgresStore) Save(ctx context.Context, in SaveInput) (FailedEvent, error) {
	evt := FailedEvent{
		EventID:     in.EventID,
		Source:      in.Source,
		Reason:      in.Reason,
		PayloadHash: hex.EncodeToString(func() []byte { sum := sha256.Sum256(in.Body); return sum[:] }()),
		FailedAt:    time.Now().UTC(),
		Headers:     in.Headers,
		Body:        in.Body,
	}

	if evt.EventID == "" {
		evt.EventID = generateID()
	}
	if evt.Headers == nil {
		evt.Headers = make(map[string]string)
	}

	headersBytes, err := json.Marshal(evt.Headers)
	if err != nil {
		return FailedEvent{}, fmt.Errorf("marshal headers: %w", err)
	}

	query := `
		INSERT INTO failed_events (event_id, source, reason, payload_hash, failed_at, headers, body)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (event_id) DO UPDATE SET
			retry_count = failed_events.retry_count + 1,
			failed_at = EXCLUDED.failed_at
		RETURNING retry_count
	`
	err = s.pool.QueryRow(ctx, query,
		evt.EventID,
		evt.Source,
		evt.Reason,
		evt.PayloadHash,
		evt.FailedAt,
		headersBytes,
		evt.Body,
	).Scan(&evt.RetryCount)
	if err != nil {
		return FailedEvent{}, fmt.Errorf("failed to save event to postgres: %w", err)
	}

	return evt, nil
}

func (s *PostgresStore) GetByID(ctx context.Context, eventID string) (FailedEvent, error) {
	query := `
		SELECT event_id, source, reason, payload_hash, failed_at, retry_count, headers, body
		FROM failed_events
		WHERE event_id = $1
	`
	var evt FailedEvent
	var headersBytes []byte

	err := s.pool.QueryRow(ctx, query, eventID).Scan(
		&evt.EventID,
		&evt.Source,
		&evt.Reason,
		&evt.PayloadHash,
		&evt.FailedAt,
		&evt.RetryCount,
		&headersBytes,
		&evt.Body,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FailedEvent{}, ErrNotFound
		}
		return FailedEvent{}, fmt.Errorf("failed to get event from postgres: %w", err)
	}
	if len(headersBytes) > 0 {
		if err := json.Unmarshal(headersBytes, &evt.Headers); err != nil {
			return FailedEvent{}, fmt.Errorf("failed to unmarshal headers: %w", err)
		}
	}
	return evt, nil
}

func (s *PostgresStore) ListRecent(ctx context.Context, limit int) ([]FailedEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	query := `
		SELECT event_id, source, reason, payload_hash, failed_at, retry_count, headers, body
		FROM failed_events
		ORDER BY failed_at DESC
		LIMIT $1
	`
	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list events from postgres: %w", err)
	}
	defer rows.Close()

	var events []FailedEvent
	for rows.Next() {
		var evt FailedEvent
		var headersBytes []byte
		if err := rows.Scan(
			&evt.EventID,
			&evt.Source,
			&evt.Reason,
			&evt.PayloadHash,
			&evt.FailedAt,
			&evt.RetryCount,
			&headersBytes,
			&evt.Body,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if len(headersBytes) > 0 {
			if err := json.Unmarshal(headersBytes, &evt.Headers); err != nil {
				return nil, fmt.Errorf("failed to unmarshal headers: %w", err)
			}
		}
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return events, nil
}
