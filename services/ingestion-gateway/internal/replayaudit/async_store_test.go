package replayaudit

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type blockingStore struct {
	entered chan struct{}
	release chan struct{}
}

func (s *blockingStore) Save(ctx context.Context, in SaveInput) (Record, error) {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	select {
	case <-s.release:
		return Record{EventID: in.EventID, Source: in.Source, Actor: in.Actor}, nil
	case <-ctx.Done():
		return Record{}, ctx.Err()
	}
}

func (s *blockingStore) List(_ context.Context, _ ListQuery) (ListResult, error) {
	return ListResult{}, nil
}

func TestAsyncStoreDropSurfacesError(t *testing.T) {
	inner := &blockingStore{entered: make(chan struct{}, 1), release: make(chan struct{})}
	a := NewAsyncStore(inner, 1)
	ctx := context.Background()

	first := make(chan error, 1)
	go func() {
		_, err := a.Save(ctx, SaveInput{EventID: "e1", Source: "github", Actor: "tester"})
		first <- err
	}()
	<-inner.entered

	second := make(chan error, 1)
	go func() {
		_, err := a.Save(ctx, SaveInput{EventID: "e2", Source: "github", Actor: "tester"})
		second <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for len(a.ch) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(a.ch) != 1 {
		t.Fatal("timed out waiting for buffered request")
	}

	if _, err := a.Save(ctx, SaveInput{EventID: "e3", Source: "github", Actor: "tester"}); !errors.Is(err, ErrAsyncDropped) {
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
	inner, err := NewFileStore(filepath.Join(t.TempDir(), "replay-audit.jsonl"))
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}
	a := NewAsyncStore(inner, 8)
	ctx := context.Background()

	rec, err := a.Save(ctx, SaveInput{EventID: "evt-1", Source: "github", Actor: "tester", Mode: "dry_run", Result: "validated"})
	if err != nil {
		t.Fatalf("async save: %v", err)
	}
	if rec.EventID != "evt-1" {
		t.Fatalf("unexpected event id %q", rec.EventID)
	}

	a.Close()
	a.Close() // idempotent
	rec2, err := a.Save(ctx, SaveInput{EventID: "evt-2", Source: "gitlab", Actor: "tester", Mode: "publish", Result: "accepted"})
	if err != nil {
		t.Fatalf("post-close save: %v", err)
	}
	if rec2.EventID != "evt-2" {
		t.Fatalf("unexpected event id %q", rec2.EventID)
	}
}
