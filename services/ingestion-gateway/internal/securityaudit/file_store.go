package securityaudit

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Record struct {
	AuditID    string    `json:"audit_id"`
	Category   string    `json:"category"`
	Outcome    string    `json:"outcome"`
	Source     string    `json:"source"`
	Reason     string    `json:"reason"`
	Path       string    `json:"path"`
	HTTPStatus int       `json:"http_status"`
	RequestID  string    `json:"request_id,omitempty"`
	Actor      string    `json:"actor,omitempty"`
	ClientIP   string    `json:"client_ip,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type SaveInput struct {
	Category   string
	Outcome    string
	Source     string
	Reason     string
	Path       string
	HTTPStatus int
	RequestID  string
	Actor      string
	ClientIP   string
}

type Store interface {
	Save(ctx context.Context, in SaveInput) (Record, error)
	ListRecent(ctx context.Context, limit int) ([]Record, error)
}

type FileStore struct {
	path          string
	retention     time.Duration
	now           func() time.Time
	pruneInterval time.Duration

	mu         sync.Mutex
	lastPruned time.Time
}

func NewFileStore(path string, retentionDays int) (*FileStore, error) {
	return newFileStore(path, retentionDays, time.Now, time.Hour)
}

func newFileStore(path string, retentionDays int, now func() time.Time, pruneInterval time.Duration) (*FileStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("security audit store path must not be empty")
	}
	if retentionDays < 1 {
		return nil, fmt.Errorf("security audit retention days must be >= 1, got %d", retentionDays)
	}
	if now == nil {
		now = time.Now
	}
	if pruneInterval <= 0 {
		pruneInterval = time.Hour
	}
	return &FileStore{
		path:          path,
		retention:     time.Duration(retentionDays) * 24 * time.Hour,
		now:           now,
		pruneInterval: pruneInterval,
	}, nil
}

func (s *FileStore) Save(ctx context.Context, in SaveInput) (Record, error) {
	record, err := s.newRecord(in)
	if err != nil {
		return Record{}, err
	}

	line, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("marshal security audit record: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return Record{}, fmt.Errorf("create security audit dir: %w", err)
	}
	if err := s.pruneExpiredLocked(ctx, record.OccurredAt); err != nil {
		return Record{}, err
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Record{}, fmt.Errorf("open security audit store: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return Record{}, fmt.Errorf("write security audit record: %w", err)
	}
	return record, nil
}

func (s *FileStore) ListRecent(ctx context.Context, limit int) ([]Record, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be > 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open security audit store: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	now := s.now().UTC()
	records := make([]Record, 0, limit)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if s.isExpired(record, now) {
			continue
		}

		if len(records) < limit {
			records = append(records, record)
			continue
		}
		copy(records, records[1:])
		records[len(records)-1] = record
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan security audit store: %w", err)
	}

	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	return records, nil
}

func (s *FileStore) newRecord(in SaveInput) (Record, error) {
	if strings.TrimSpace(in.Category) == "" {
		return Record{}, errors.New("category must not be empty")
	}
	if strings.TrimSpace(in.Outcome) == "" {
		return Record{}, errors.New("outcome must not be empty")
	}
	if strings.TrimSpace(in.Source) == "" {
		return Record{}, errors.New("source must not be empty")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return Record{}, errors.New("reason must not be empty")
	}
	if strings.TrimSpace(in.Path) == "" {
		return Record{}, errors.New("path must not be empty")
	}
	if in.HTTPStatus < 100 || in.HTTPStatus > 599 {
		return Record{}, fmt.Errorf("http status must be between 100 and 599, got %d", in.HTTPStatus)
	}

	auditID, err := newAuditID()
	if err != nil {
		return Record{}, err
	}
	return Record{
		AuditID:    auditID,
		Category:   strings.TrimSpace(in.Category),
		Outcome:    strings.TrimSpace(in.Outcome),
		Source:     strings.TrimSpace(in.Source),
		Reason:     strings.TrimSpace(in.Reason),
		Path:       strings.TrimSpace(in.Path),
		HTTPStatus: in.HTTPStatus,
		RequestID:  strings.TrimSpace(in.RequestID),
		Actor:      strings.TrimSpace(in.Actor),
		ClientIP:   strings.TrimSpace(in.ClientIP),
		OccurredAt: s.now().UTC(),
	}, nil
}

func (s *FileStore) pruneExpiredLocked(ctx context.Context, now time.Time) error {
	if !s.lastPruned.IsZero() && now.Sub(s.lastPruned) < s.pruneInterval {
		return nil
	}

	records, err := s.readRecordsLocked(ctx)
	if err != nil {
		return err
	}

	filtered := make([]Record, 0, len(records))
	for _, record := range records {
		if s.isExpired(record, now) {
			continue
		}
		filtered = append(filtered, record)
	}

	if len(filtered) == len(records) && fileExists(s.path) {
		s.lastPruned = now
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create security audit dir: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open security audit store for pruning: %w", err)
	}

	writeErr := func() error {
		defer func() {
			_ = f.Close()
		}()
		enc := json.NewEncoder(f)
		for _, record := range filtered {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := enc.Encode(record); err != nil {
				return fmt.Errorf("rewrite security audit store: %w", err)
			}
		}
		return nil
	}()
	if writeErr != nil {
		return writeErr
	}
	s.lastPruned = now
	return nil
}

func (s *FileStore) readRecordsLocked(ctx context.Context) ([]Record, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open security audit store: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	records := make([]Record, 0, 128)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan security audit store: %w", err)
	}
	return records, nil
}

func (s *FileStore) isExpired(record Record, now time.Time) bool {
	cutoff := now.Add(-s.retention)
	return record.OccurredAt.Before(cutoff)
}

func newAuditID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate security audit id: %w", err)
	}
	return "sa_" + hex.EncodeToString(b), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
