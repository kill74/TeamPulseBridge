package replayaudit

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
	"sort"
	"strings"
	"sync"
	"time"
)

type Record struct {
	AuditID    string    `json:"audit_id"`
	EventID    string    `json:"event_id"`
	Source     string    `json:"source"`
	Actor      string    `json:"actor"`
	Mode       string    `json:"mode"`
	Result     string    `json:"result"`
	ErrorCode  string    `json:"error_code,omitempty"`
	HTTPStatus int       `json:"http_status"`
	RequestID  string    `json:"request_id,omitempty"`
	ReplayedAt time.Time `json:"replayed_at"`
}

type SaveInput struct {
	EventID    string
	Source     string
	Actor      string
	Mode       string
	Result     string
	ErrorCode  string
	HTTPStatus int
	RequestID  string
}

type SortOrder string

const (
	SortDesc SortOrder = "desc"
	SortAsc  SortOrder = "asc"
)

var (
	ErrInvalidListQuery = errors.New("invalid replay audit list query")
	ErrCursorNotFound   = errors.New("replay audit cursor not found")
)

type ListQuery struct {
	Limit   int
	Cursor  string
	Actor   string
	Result  string
	EventID string
	Sort    SortOrder
}

type ListResult struct {
	Records    []Record `json:"records"`
	HasMore    bool     `json:"has_more"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

type Store interface {
	Save(ctx context.Context, in SaveInput) (Record, error)
	List(ctx context.Context, q ListQuery) (ListResult, error)
}

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) (*FileStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("replay audit store path must not be empty")
	}
	return &FileStore{path: path}, nil
}

func (s *FileStore) Save(_ context.Context, in SaveInput) (Record, error) {
	if strings.TrimSpace(in.EventID) == "" {
		return Record{}, errors.New("event id must not be empty")
	}
	if strings.TrimSpace(in.Actor) == "" {
		return Record{}, errors.New("actor must not be empty")
	}
	if strings.TrimSpace(in.Mode) == "" {
		return Record{}, errors.New("mode must not be empty")
	}
	if strings.TrimSpace(in.Result) == "" {
		return Record{}, errors.New("result must not be empty")
	}

	auditID, err := newAuditID()
	if err != nil {
		return Record{}, err
	}
	record := Record{
		AuditID:    auditID,
		EventID:    strings.TrimSpace(in.EventID),
		Source:     strings.TrimSpace(in.Source),
		Actor:      strings.TrimSpace(in.Actor),
		Mode:       strings.TrimSpace(in.Mode),
		Result:     strings.TrimSpace(in.Result),
		ErrorCode:  strings.TrimSpace(in.ErrorCode),
		HTTPStatus: in.HTTPStatus,
		RequestID:  strings.TrimSpace(in.RequestID),
		ReplayedAt: time.Now().UTC(),
	}

	line, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("marshal replay audit record: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return Record{}, fmt.Errorf("create replay audit dir: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Record{}, fmt.Errorf("open replay audit store: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return Record{}, fmt.Errorf("write replay audit record: %w", err)
	}
	return record, nil
}

func (s *FileStore) List(ctx context.Context, q ListQuery) (ListResult, error) {
	q, err := normalizeListQuery(q)
	if err != nil {
		return ListResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return ListResult{Records: []Record{}}, nil
	}
	if err != nil {
		return ListResult{}, fmt.Errorf("open replay audit store: %w", err)
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
			return ListResult{}, ctx.Err()
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
		if !matchesRecord(record, q) {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return ListResult{}, fmt.Errorf("scan replay audit store: %w", err)
	}

	sortRecords(records, q.Sort)

	start := 0
	if q.Cursor != "" {
		index := -1
		for i := range records {
			if records[i].AuditID == q.Cursor {
				index = i
				break
			}
		}
		if index == -1 {
			return ListResult{}, fmt.Errorf("%w: %s", ErrCursorNotFound, q.Cursor)
		}
		start = index + 1
	}

	if start >= len(records) {
		return ListResult{Records: []Record{}}, nil
	}

	end := start + q.Limit
	if end > len(records) {
		end = len(records)
	}
	page := make([]Record, end-start)
	copy(page, records[start:end])

	result := ListResult{
		Records: page,
		HasMore: end < len(records),
	}
	if result.HasMore && len(page) > 0 {
		result.NextCursor = page[len(page)-1].AuditID
	}
	return result, nil
}

func (s *FileStore) ListRecent(ctx context.Context, limit int) ([]Record, error) {
	out, err := s.List(ctx, ListQuery{
		Limit: limit,
		Sort:  SortDesc,
	})
	if err != nil {
		return nil, err
	}
	return out.Records, nil
}

func ParseSortOrder(raw string) (SortOrder, error) {
	normalized := SortOrder(strings.ToLower(strings.TrimSpace(raw)))
	if normalized == "" {
		return SortDesc, nil
	}
	if normalized != SortDesc && normalized != SortAsc {
		return "", fmt.Errorf("%w: sort must be asc or desc", ErrInvalidListQuery)
	}
	return normalized, nil
}

func normalizeListQuery(q ListQuery) (ListQuery, error) {
	if q.Limit <= 0 {
		return ListQuery{}, fmt.Errorf("%w: limit must be > 0", ErrInvalidListQuery)
	}
	sortOrder, err := ParseSortOrder(string(q.Sort))
	if err != nil {
		return ListQuery{}, err
	}
	q.Sort = sortOrder
	q.Cursor = strings.TrimSpace(q.Cursor)
	q.Actor = strings.TrimSpace(q.Actor)
	q.Result = strings.TrimSpace(q.Result)
	q.EventID = strings.TrimSpace(q.EventID)
	return q, nil
}

func matchesRecord(record Record, q ListQuery) bool {
	if q.Actor != "" && !strings.EqualFold(strings.TrimSpace(record.Actor), q.Actor) {
		return false
	}
	if q.Result != "" && !strings.EqualFold(strings.TrimSpace(record.Result), q.Result) {
		return false
	}
	if q.EventID != "" && !strings.EqualFold(strings.TrimSpace(record.EventID), q.EventID) {
		return false
	}
	return true
}

func sortRecords(records []Record, order SortOrder) {
	sort.Slice(records, func(i, j int) bool {
		left := records[i]
		right := records[j]
		if left.ReplayedAt.Equal(right.ReplayedAt) {
			if order == SortAsc {
				return left.AuditID < right.AuditID
			}
			return left.AuditID > right.AuditID
		}
		if order == SortAsc {
			return left.ReplayedAt.Before(right.ReplayedAt)
		}
		return left.ReplayedAt.After(right.ReplayedAt)
	})
}

func newAuditID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate audit id: %w", err)
	}
	return "ra_" + hex.EncodeToString(b), nil
}
