package failstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// blockingStore blocks inside Save until release is closed, so tests can
// deterministically saturate the async queue (busy worker + full buffer).
type blockingStore struct {
	entered chan struct{}
	release chan struct{}
}

func (s *blockingStore) Save(ctx context.Context, in SaveInput) (FailedEvent, error) {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	select {
	case <-s.release:
		return FailedEvent{EventID: in.EventID, Source: in.Source, Reason: in.Reason}, nil
	case <-ctx.Done():
		return FailedEvent{}, ctx.Err()
	}
}

func (s *blockingStore) GetByID(_ context.Context, _ string) (FailedEvent, error) {
	return FailedEvent{}, errors.New("not found")
}

func (s *blockingStore) ListRecent(_ context.Context, _ int) ([]FailedEvent, error) {
	return nil, nil
}

func (s *blockingStore) Delete(_ context.Context, _ string) error { return nil }

func (s *blockingStore) UpdateRetryCount(_ context.Context, _ string, _ int) error { return nil }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAsyncStoreDropSurfacesError(t *testing.T) {
	inner := &blockingStore{entered: make(chan struct{}, 1), release: make(chan struct{})}
	a := NewAsyncStore(inner, 1)
	ctx := context.Background()

	first := make(chan error, 1)
	go func() {
		_, err := a.Save(ctx, SaveInput{EventID: "e1", Source: "github", Reason: "r1"})
		first <- err
	}()
	<-inner.entered // worker is now blocked inside inner.Save

	second := make(chan error, 1)
	go func() {
		_, err := a.Save(ctx, SaveInput{EventID: "e2", Source: "github", Reason: "r2"})
		second <- err
	}()
	waitFor(t, "buffered request", func() bool { return len(a.ch) == 1 })

	// Worker busy + buffer full: must report the loss, not fake success.
	if _, err := a.Save(ctx, SaveInput{EventID: "e3", Source: "github", Reason: "r3"}); !errors.Is(err, ErrAsyncDropped) {
		t.Fatalf("expected ErrAsyncDropped, got %v", err)
	}
	if got := a.Dropped(); got != 1 {
		t.Fatalf("expected 1 dropped record, got %d", got)
	}

	close(inner.release)
	if err := <-first; err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second save: %v", err)
	}
}

func TestAsyncStoreSaveThroughAndCloseFallback(t *testing.T) {
	inner, err := NewFileStore(filepath.Join(t.TempDir(), "failed-events.jsonl"))
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}
	a := NewAsyncStore(inner, 8)
	ctx := context.Background()

	rec, err := a.Save(ctx, SaveInput{
		EventID: "evt-async-1",
		Source:  "github",
		Reason:  "ERR_PUBLISH_FAILED",
		Body:    []byte(`{"action":"opened"}`),
	})
	if err != nil {
		t.Fatalf("async save: %v", err)
	}
	if rec.EventID != "evt-async-1" {
		t.Fatalf("unexpected event id %q", rec.EventID)
	}

	// After Close, saves fall back to synchronous persistence.
	a.Close()
	a.Close() // idempotent
	rec2, err := a.Save(ctx, SaveInput{
		EventID: "evt-async-2",
		Source:  "gitlab",
		Reason:  "ERR_PUBLISH_FAILED",
		Body:    []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("post-close save: %v", err)
	}
	if rec2.EventID != "evt-async-2" {
		t.Fatalf("unexpected event id %q", rec2.EventID)
	}
}
