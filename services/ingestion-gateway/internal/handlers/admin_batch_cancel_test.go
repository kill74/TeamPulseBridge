package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
)

// A pre-cancelled batch must return a well-formed partial response without
// deadlocking in-flight workers and without leaking zero-value entries.
func TestAdminReplayFailedEventsBatchCancelledContext(t *testing.T) {
	store := &adminStoreStub{events: map[string]failstore.FailedEvent{}}
	ids := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("evt_cancel_%d", i)
		ids = append(ids, id)
		store.events[id] = failstore.FailedEvent{
			EventID: id,
			Source:  "github",
			Headers: map[string]string{"X-Test": "1"},
			Body:    json.RawMessage(`{"action":"opened"}`),
		}
	}
	h := NewAdminHandlerWithDependencies(config.Config{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), store, &adminAuditStub{}, &adminSecurityAuditStub{}, nil)

	body, _ := json.Marshal(map[string]any{"event_ids": ids, "dry_run": true})
	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // cancel before dispatch
	req = req.WithContext(ctx)

	done := make(chan int, 1)
	rr := httptest.NewRecorder()
	go func() {
		h.ReplayFailedEventsBatch(rr, req)
		done <- rr.Code
	}()
	select {
	case code := <-done:
		if code != http.StatusOK && code != http.StatusMultiStatus && code != http.StatusBadRequest {
			t.Fatalf("unexpected status %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("batch handler deadlocked on cancelled context")
	}

	var payload struct {
		Summary struct {
			Requested int `json:"requested"`
			Processed int `json:"processed"`
		} `json:"summary"`
		Results []struct {
			EventID string `json:"event_id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Summary.Requested != len(ids) {
		t.Fatalf("requested = %d, want %d", payload.Summary.Requested, len(ids))
	}
	if len(payload.Results) != payload.Summary.Processed {
		t.Fatalf("results len %d != processed %d", len(payload.Results), payload.Summary.Processed)
	}
	for _, r := range payload.Results {
		if strings.TrimSpace(r.EventID) == "" {
			t.Fatal("zero-value entry leaked into batch results")
		}
	}
}
