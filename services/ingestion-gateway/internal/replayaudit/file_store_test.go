package replayaudit

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestFileStoreSaveAndListRecent(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "replay-audit.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	for i := 1; i <= 5; i++ {
		_, err := store.Save(context.Background(), SaveInput{
			EventID:    "evt_" + strconv.Itoa(i),
			Source:     "github",
			Actor:      "dev@example.com",
			Mode:       "publish",
			Result:     "accepted",
			HTTPStatus: 202,
		})
		if err != nil {
			t.Fatalf("save replay audit %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	recent, err := store.List(context.Background(), ListQuery{
		Limit: 3,
		Sort:  SortDesc,
	})
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recent.Records))
	}
	if recent.Records[0].EventID != "evt_5" || recent.Records[1].EventID != "evt_4" || recent.Records[2].EventID != "evt_3" {
		t.Fatalf("unexpected replay audit order: %+v", []string{recent.Records[0].EventID, recent.Records[1].EventID, recent.Records[2].EventID})
	}
	if !recent.HasMore {
		t.Fatal("expected has_more=true for paged response")
	}
	if recent.NextCursor == "" {
		t.Fatal("expected next cursor for paged response")
	}
}

func TestFileStoreListRecentMissingFileReturnsEmpty(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "missing-replay-audit.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	recent, err := store.List(context.Background(), ListQuery{
		Limit: 10,
		Sort:  SortDesc,
	})
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent.Records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(recent.Records))
	}
}

func TestFileStoreListSupportsFiltersAndCursorPaging(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "replay-audit.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	cases := []SaveInput{
		{EventID: "evt_1", Source: "github", Actor: "alice@example.com", Mode: "publish", Result: "validated", HTTPStatus: 200},
		{EventID: "evt_2", Source: "github", Actor: "bob@example.com", Mode: "publish", Result: "accepted", HTTPStatus: 202},
		{EventID: "evt_3", Source: "github", Actor: "alice@example.com", Mode: "publish", Result: "accepted", HTTPStatus: 202},
		{EventID: "evt_4", Source: "github", Actor: "alice@example.com", Mode: "publish", Result: "failed", HTTPStatus: 503},
		{EventID: "evt_5", Source: "github", Actor: "alice@example.com", Mode: "publish", Result: "accepted", HTTPStatus: 202},
	}
	for i := range cases {
		if _, err := store.Save(context.Background(), cases[i]); err != nil {
			t.Fatalf("save replay audit %d: %v", i+1, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	firstPage, err := store.List(context.Background(), ListQuery{
		Limit:  1,
		Actor:  "ALICE@EXAMPLE.COM",
		Result: "accepted",
		Sort:   SortDesc,
	})
	if err != nil {
		t.Fatalf("first page list: %v", err)
	}
	if len(firstPage.Records) != 1 || firstPage.Records[0].EventID != "evt_5" {
		t.Fatalf("unexpected first page records: %+v", firstPage.Records)
	}
	if !firstPage.HasMore || firstPage.NextCursor == "" {
		t.Fatalf("expected has_more=true and next_cursor set, got %+v", firstPage)
	}

	secondPage, err := store.List(context.Background(), ListQuery{
		Limit:  1,
		Cursor: firstPage.NextCursor,
		Actor:  "alice@example.com",
		Result: "accepted",
		Sort:   SortDesc,
	})
	if err != nil {
		t.Fatalf("second page list: %v", err)
	}
	if len(secondPage.Records) != 1 || secondPage.Records[0].EventID != "evt_3" {
		t.Fatalf("unexpected second page records: %+v", secondPage.Records)
	}
	if secondPage.HasMore {
		t.Fatalf("expected last page has_more=false, got %+v", secondPage)
	}
}

func TestFileStoreListUnknownCursorReturnsError(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "replay-audit.jsonl")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := store.Save(context.Background(), SaveInput{
		EventID: "evt_1", Source: "github", Actor: "dev@example.com", Mode: "publish", Result: "accepted", HTTPStatus: 202,
	}); err != nil {
		t.Fatalf("save replay audit: %v", err)
	}

	_, err = store.List(context.Background(), ListQuery{
		Limit:  5,
		Cursor: "ra_missing",
		Sort:   SortDesc,
	})
	if !errors.Is(err, ErrCursorNotFound) {
		t.Fatalf("expected ErrCursorNotFound, got %v", err)
	}
}

func TestParseSortOrderRejectsInvalidValue(t *testing.T) {
	_, err := ParseSortOrder("latest")
	if !errors.Is(err, ErrInvalidListQuery) {
		t.Fatalf("expected ErrInvalidListQuery, got %v", err)
	}
}
