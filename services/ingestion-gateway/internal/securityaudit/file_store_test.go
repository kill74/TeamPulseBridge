package securityaudit

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStoreSaveAndListRecent(t *testing.T) {
	now := time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)
	clk := &auditClock{t: now}

	store, err := newFileStore(filepath.Join(t.TempDir(), "security-audit.jsonl"), 30, clk.Now, time.Minute)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	for i := 0; i < 3; i++ {
		_, err := store.Save(context.Background(), SaveInput{
			Category:   "request_rejected",
			Outcome:    "rejected",
			Source:     "github",
			Reason:     "webhook_auth_failed",
			Path:       "/webhooks/github",
			HTTPStatus: 401,
			RequestID:  "req-test",
			ClientIP:   "198.51.100.10",
		})
		if err != nil {
			t.Fatalf("save record %d: %v", i+1, err)
		}
		clk.Add(time.Second)
	}

	recent, err := store.ListRecent(context.Background(), 2)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recent))
	}
	if !recent[0].OccurredAt.After(recent[1].OccurredAt) {
		t.Fatalf("expected most recent record first, got %+v", recent)
	}
}

func TestFileStorePrunesExpiredRecordsOnSave(t *testing.T) {
	now := time.Date(2026, time.January, 10, 9, 0, 0, 0, time.UTC)
	clk := &auditClock{t: now}

	storePath := filepath.Join(t.TempDir(), "security-audit.jsonl")
	store, err := newFileStore(storePath, 1, clk.Now, time.Nanosecond)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if _, err := store.Save(context.Background(), SaveInput{
		Category:   "request_rejected",
		Outcome:    "rejected",
		Source:     "admin",
		Reason:     "admin_jwt_invalid",
		Path:       "/admin/configz",
		HTTPStatus: 401,
	}); err != nil {
		t.Fatalf("save initial record: %v", err)
	}

	clk.Add(48 * time.Hour)

	if _, err := store.Save(context.Background(), SaveInput{
		Category:   "request_rejected",
		Outcome:    "rejected",
		Source:     "admin",
		Reason:     "rate_limit_exceeded",
		Path:       "/admin/configz",
		HTTPStatus: 429,
	}); err != nil {
		t.Fatalf("save replacement record: %v", err)
	}

	recent, err := store.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 unexpired record, got %d", len(recent))
	}
	if recent[0].Reason != "rate_limit_exceeded" {
		t.Fatalf("expected pruned store to keep newest reason, got %q", recent[0].Reason)
	}
}

type auditClock struct {
	t time.Time
}

func (c *auditClock) Now() time.Time {
	return c.t
}

func (c *auditClock) Add(d time.Duration) {
	c.t = c.t.Add(d)
}
