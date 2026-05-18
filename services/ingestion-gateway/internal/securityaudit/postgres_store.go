package securityaudit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		CREATE TABLE IF NOT EXISTS security_audit (
			audit_id VARCHAR(64) PRIMARY KEY,
			category VARCHAR(50) NOT NULL,
			outcome VARCHAR(50) NOT NULL,
			source VARCHAR(255) NOT NULL,
			reason TEXT NOT NULL,
			path TEXT NOT NULL,
			http_status INT NOT NULL,
			request_id VARCHAR(64) NOT NULL,
			actor VARCHAR(255) NOT NULL,
			client_ip VARCHAR(255) NOT NULL,
			occurred_at TIMESTAMP WITH TIME ZONE NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_security_audit_occurred_at ON security_audit(occurred_at DESC);
	`
	_, _ = pool.Exec(ctx, query)

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

func (s *PostgresStore) Save(ctx context.Context, in SaveInput) (Record, error) {
	rec := Record{
		AuditID:    generateID(),
		Category:   in.Category,
		Outcome:    in.Outcome,
		Source:     in.Source,
		Reason:     in.Reason,
		Path:       in.Path,
		HTTPStatus: in.HTTPStatus,
		RequestID:  in.RequestID,
		Actor:      in.Actor,
		ClientIP:   in.ClientIP,
		OccurredAt: time.Now().UTC(),
	}

	query := `
		INSERT INTO security_audit (audit_id, category, outcome, source, reason, path, http_status, request_id, actor, client_ip, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := s.pool.Exec(ctx, query,
		rec.AuditID,
		rec.Category,
		rec.Outcome,
		rec.Source,
		rec.Reason,
		rec.Path,
		rec.HTTPStatus,
		rec.RequestID,
		rec.Actor,
		rec.ClientIP,
		rec.OccurredAt,
	)
	if err != nil {
		return Record{}, fmt.Errorf("failed to save security audit to postgres: %w", err)
	}

	return rec, nil
}

func (s *PostgresStore) ListRecent(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	query := `
		SELECT audit_id, category, outcome, source, reason, path, http_status, request_id, actor, client_ip, occurred_at
		FROM security_audit
		ORDER BY occurred_at DESC
		LIMIT $1
	`
	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list security audits from postgres: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(
			&rec.AuditID,
			&rec.Category,
			&rec.Outcome,
			&rec.Source,
			&rec.Reason,
			&rec.Path,
			&rec.HTTPStatus,
			&rec.RequestID,
			&rec.Actor,
			&rec.ClientIP,
			&rec.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan security audit: %w", err)
		}
		records = append(records, rec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return records, nil
}
