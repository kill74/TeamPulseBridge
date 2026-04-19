package failstore

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
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
