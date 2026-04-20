package failstore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFileStoreSaveAndGetByID(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "failed-events.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	saved, err := store.Save(context.Background(), SaveInput{
		EventID: "evt_123",
		Source:  "github",
		Reason:  "ERR_PUBLISH_FAILED",
		Headers: map[string]string{"X-Request-Id": "req1"},
		Body:    []byte(`{"action":"opened"}`),
	})
	if err != nil {
		t.Fatalf("save failed event: %v", err)
	}
	if saved.EventID != "evt_123" {
		t.Fatalf("unexpected event id: %s", saved.EventID)
	}
	if saved.PayloadHash == "" {
		t.Fatal("payload hash should be set")
	}

	found, err := store.GetByID(context.Background(), "evt_123")
	if err != nil {
		t.Fatalf("get failed event: %v", err)
	}
	if found.Source != "github" {
		t.Fatalf("unexpected source: %s", found.Source)
	}
	if !json.Valid(found.Body) {
		t.Fatal("body should be valid json")
	}
}

func TestFileStoreGetByIDNotFound(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "failed-events.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	_, err = store.GetByID(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFileStoreGetByIDHonorsContextCancellation(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "failed-events.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := store.Save(context.Background(), SaveInput{
			EventID: "evt_" + strconv.Itoa(i),
			Source:  "github",
			Reason:  "ERR_PUBLISH_FAILED",
			Body:    []byte(`{"id":` + strconv.Itoa(i) + `}`),
		}); err != nil {
			t.Fatalf("save %d failed: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.GetByID(ctx, "evt_4")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestFileStoreListRecent(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "failed-events.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	for i := 1; i <= 5; i++ {
		_, err := store.Save(context.Background(), SaveInput{
			EventID: "evt_" + strconv.Itoa(i),
			Source:  "github",
			Reason:  "ERR_PUBLISH_FAILED",
			Body:    []byte(`{"id":` + strconv.Itoa(i) + `}`),
		})
		if err != nil {
			t.Fatalf("save %d failed: %v", i, err)
		}
	}

	recent, err := store.ListRecent(context.Background(), 3)
	if err != nil {
		t.Fatalf("list recent failed: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 events, got %d", len(recent))
	}
	if recent[0].EventID != "evt_5" || recent[1].EventID != "evt_4" || recent[2].EventID != "evt_3" {
		t.Fatalf("unexpected event order: %+v", []string{recent[0].EventID, recent[1].EventID, recent[2].EventID})
	}
}

func TestFileStoreListRecentMissingFileReturnsEmpty(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "missing.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	recent, err := store.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("list recent failed: %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("expected empty recent list, got %d", len(recent))
	}
}
