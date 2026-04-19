package failstore

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
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

var ErrNotFound = errors.New("failed event not found")

type FailedEvent struct {
	EventID     string            `json:"event_id"`
	Source      string            `json:"source"`
	Reason      string            `json:"reason"`
	PayloadHash string            `json:"payload_hash"`
	FailedAt    time.Time         `json:"failed_at"`
	Headers     map[string]string `json:"headers"`
	Body        json.RawMessage   `json:"body"`
}

type SaveInput struct {
	EventID string
	Source  string
	Reason  string
	Headers map[string]string
	Body    []byte
}

type Store interface {
	Save(ctx context.Context, in SaveInput) (FailedEvent, error)
	GetByID(ctx context.Context, eventID string) (FailedEvent, error)
}

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) (*FileStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("failed event store path must not be empty")
	}
	return &FileStore{path: path}, nil
}

func (s *FileStore) Save(_ context.Context, in SaveInput) (FailedEvent, error) {
	if strings.TrimSpace(in.Source) == "" {
		return FailedEvent{}, errors.New("source must not be empty")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return FailedEvent{}, errors.New("reason must not be empty")
	}
	if len(in.Body) == 0 {
		return FailedEvent{}, errors.New("body must not be empty")
	}
	if !json.Valid(in.Body) {
		return FailedEvent{}, errors.New("body must be valid JSON")
	}

	eventID := strings.TrimSpace(in.EventID)
	if eventID == "" {
		id, err := newEventID()
		if err != nil {
			return FailedEvent{}, err
		}
		eventID = id
	}

	record := FailedEvent{
		EventID:     eventID,
		Source:      in.Source,
		Reason:      in.Reason,
		PayloadHash: hashBody(in.Body),
		FailedAt:    time.Now().UTC(),
		Headers:     cloneHeaders(in.Headers),
		Body:        append([]byte(nil), in.Body...),
	}

	line, err := json.Marshal(record)
	if err != nil {
		return FailedEvent{}, fmt.Errorf("marshal failed event: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return FailedEvent{}, fmt.Errorf("create failed event store dir: %w", err)
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return FailedEvent{}, fmt.Errorf("open failed event store: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return FailedEvent{}, fmt.Errorf("write failed event: %w", err)
	}

	return record, nil
}

func (s *FileStore) GetByID(_ context.Context, eventID string) (FailedEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return FailedEvent{}, errors.New("event id must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return FailedEvent{}, ErrNotFound
	}
	if err != nil {
		return FailedEvent{}, fmt.Errorf("open failed event store: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event FailedEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.EventID == eventID {
			return event, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return FailedEvent{}, fmt.Errorf("scan failed event store: %w", err)
	}
	return FailedEvent{}, ErrNotFound
}

func hashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func cloneHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func newEventID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate event id: %w", err)
	}
	return "fev_" + hex.EncodeToString(b), nil
}
