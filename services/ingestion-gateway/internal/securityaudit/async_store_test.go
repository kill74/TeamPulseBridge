package securityaudit

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAsyncStoreSaveThroughAndCloseFallback(t *testing.T) {
	inner, err := NewFileStore(filepath.Join(t.TempDir(), "security-audit.jsonl"), 30)
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}
	a := NewAsyncStore(inner, 8)
	ctx := context.Background()

	rec, err := a.Save(ctx, SaveInput{
		Category:   "request_rejected",
		Outcome:    "rejected",
		Source:     "github",
		Reason:     "rate_limit_exceeded",
		Path:       "/webhooks/github",
		HTTPStatus: 429,
	})
	if err != nil {
		t.Fatalf("async save: %v", err)
	}
	if rec.Reason != "rate_limit_exceeded" {
		t.Fatalf("unexpected reason %q", rec.Reason)
	}

	a.Close()
	a.Close() // idempotent
	rec2, err := a.Save(ctx, SaveInput{
		Category:   "request_rejected",
		Outcome:    "rejected",
		Source:     "admin",
		Reason:     "auth_failed",
		Path:       "/admin/flags",
		HTTPStatus: 401,
	})
	if err != nil {
		t.Fatalf("post-close save: %v", err)
	}
	if rec2.Reason != "auth_failed" {
		t.Fatalf("unexpected reason %q", rec2.Reason)
	}
	if got := a.Dropped(); got != 0 {
		t.Fatalf("expected 0 dropped records, got %d", got)
	}
}
