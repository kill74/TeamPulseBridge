package replayaudit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) (*PostgresStore, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `
		CREATE TABLE IF NOT EXISTS replay_audit (
			audit_id VARCHAR(64) PRIMARY KEY,
			event_id VARCHAR(64) NOT NULL,
			source VARCHAR(255) NOT NULL,
			actor VARCHAR(255) NOT NULL,
			mode VARCHAR(50) NOT NULL,
			result VARCHAR(50) NOT NULL,
			error_code VARCHAR(100) NOT NULL,
			http_status INT NOT NULL,
			request_id VARCHAR(64) NOT NULL,
			replayed_at TIMESTAMP WITH TIME ZONE NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_replay_audit_replayed_at ON replay_audit(replayed_at DESC);
	`
	if _, err := pool.Exec(ctx, query); err != nil {
		return nil, fmt.Errorf("create replay audit tables: %w", err)
	}

	return &PostgresStore{
		pool: pool,
	}, nil
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
		EventID:    in.EventID,
		Source:     in.Source,
		Actor:      in.Actor,
		Mode:       in.Mode,
		Result:     in.Result,
		ErrorCode:  in.ErrorCode,
		HTTPStatus: in.HTTPStatus,
		RequestID:  in.RequestID,
		ReplayedAt: time.Now().UTC(),
	}

	query := `
		INSERT INTO replay_audit (audit_id, event_id, source, actor, mode, result, error_code, http_status, request_id, replayed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := s.pool.Exec(ctx, query,
		rec.AuditID,
		rec.EventID,
		rec.Source,
		rec.Actor,
		rec.Mode,
		rec.Result,
		rec.ErrorCode,
		rec.HTTPStatus,
		rec.RequestID,
		rec.ReplayedAt,
	)
	if err != nil {
		return Record{}, fmt.Errorf("failed to save replay audit to postgres: %w", err)
	}

	return rec, nil
}

func (s *PostgresStore) List(ctx context.Context, q ListQuery) (ListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 50
	}
	if q.Limit > 1000 {
		q.Limit = 1000
	}

	query := `SELECT audit_id, event_id, source, actor, mode, result, error_code, http_status, request_id, replayed_at FROM replay_audit WHERE 1=1`
	args := []any{}
	paramID := 1

	if q.Actor != "" {
		query += fmt.Sprintf(" AND actor = $%d", paramID)
		args = append(args, q.Actor)
		paramID++
	}
	if q.Result != "" {
		query += fmt.Sprintf(" AND result = $%d", paramID)
		args = append(args, q.Result)
		paramID++
	}
	if q.EventID != "" {
		query += fmt.Sprintf(" AND event_id = $%d", paramID)
		args = append(args, q.EventID)
		paramID++
	}

	if q.Cursor != "" {
		cursorParts := strings.SplitN(q.Cursor, "|", 2)
		if len(cursorParts) == 2 {
			cursorTime, err := time.Parse(time.RFC3339Nano, cursorParts[0])
			if err == nil {
				if q.Sort == SortAsc {
					query += fmt.Sprintf(" AND (replayed_at, audit_id) > ($%d, $%d)", paramID, paramID+1)
				} else {
					query += fmt.Sprintf(" AND (replayed_at, audit_id) < ($%d, $%d)", paramID, paramID+1)
				}
				args = append(args, cursorTime, cursorParts[1])
				paramID += 2
			}
		}
	}

	if q.Sort == SortAsc {
		query += ` ORDER BY replayed_at ASC, audit_id ASC`
	} else {
		query += ` ORDER BY replayed_at DESC, audit_id DESC`
	}

	query += fmt.Sprintf(" LIMIT $%d", paramID)
	args = append(args, q.Limit+1)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("list replay audits from postgres: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		var rec Record
		if err := rows.Scan(
			&rec.AuditID,
			&rec.EventID,
			&rec.Source,
			&rec.Actor,
			&rec.Mode,
			&rec.Result,
			&rec.ErrorCode,
			&rec.HTTPStatus,
			&rec.RequestID,
			&rec.ReplayedAt,
		); err != nil {
			return ListResult{}, fmt.Errorf("scan replay audit: %w", err)
		}
		records = append(records, rec)
	}

	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("rows error: %w", err)
	}

	var nextCursor string
	hasMore := false
	if len(records) > q.Limit {
		hasMore = true
		records = records[:q.Limit]
	}
	if hasMore && len(records) > 0 {
		last := records[len(records)-1]
		nextCursor = last.ReplayedAt.Format(time.RFC3339Nano) + "|" + last.AuditID
	}

	return ListResult{
		Records:    records,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}
