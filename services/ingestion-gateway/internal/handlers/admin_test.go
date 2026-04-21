package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
	"teampulsebridge/services/ingestion-gateway/internal/replayaudit"
	"teampulsebridge/services/ingestion-gateway/internal/securityaudit"
)

type adminStoreStub struct {
	events  map[string]failstore.FailedEvent
	recent  []failstore.FailedEvent
	getErr  error
	listErr error
}

func (s *adminStoreStub) Save(_ context.Context, in failstore.SaveInput) (failstore.FailedEvent, error) {
	event := failstore.FailedEvent{
		EventID: in.EventID,
		Source:  in.Source,
		Reason:  in.Reason,
		Headers: in.Headers,
		Body:    append([]byte(nil), in.Body...),
	}
	if s.events == nil {
		s.events = make(map[string]failstore.FailedEvent)
	}
	s.events[event.EventID] = event
	return event, nil
}

func (s *adminStoreStub) GetByID(_ context.Context, eventID string) (failstore.FailedEvent, error) {
	if s.getErr != nil {
		return failstore.FailedEvent{}, s.getErr
	}
	if event, ok := s.events[eventID]; ok {
		return event, nil
	}
	return failstore.FailedEvent{}, failstore.ErrNotFound
}

func (s *adminStoreStub) ListRecent(_ context.Context, limit int) ([]failstore.FailedEvent, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if limit <= 0 || len(s.recent) == 0 {
		return []failstore.FailedEvent{}, nil
	}
	if limit > len(s.recent) {
		limit = len(s.recent)
	}
	out := make([]failstore.FailedEvent, limit)
	copy(out, s.recent[:limit])
	return out, nil
}

type adminPublisherStub struct {
	calls int
	last  struct {
		source  string
		body    []byte
		headers map[string]string
	}
	err error
}

func (s *adminPublisherStub) Publish(_ context.Context, source string, body []byte, headers map[string]string) error {
	s.calls++
	s.last.source = source
	s.last.body = append([]byte(nil), body...)
	s.last.headers = make(map[string]string, len(headers))
	for k, v := range headers {
		s.last.headers[k] = v
	}
	return s.err
}

func (s *adminPublisherStub) Close() error { return nil }

type adminAuditStub struct {
	calls     int
	saved     []replayaudit.SaveInput
	recent    []replayaudit.Record
	saveErr   error
	listErr   error
	lastQuery replayaudit.ListQuery
}

func (s *adminAuditStub) Save(_ context.Context, in replayaudit.SaveInput) (replayaudit.Record, error) {
	if s.saveErr != nil {
		return replayaudit.Record{}, s.saveErr
	}
	s.calls++
	s.saved = append(s.saved, in)
	return replayaudit.Record{
		EventID:    in.EventID,
		Source:     in.Source,
		Actor:      in.Actor,
		Mode:       in.Mode,
		Result:     in.Result,
		ErrorCode:  in.ErrorCode,
		HTTPStatus: in.HTTPStatus,
		RequestID:  in.RequestID,
	}, nil
}

type adminSecurityAuditStub struct {
	listErr error
	recent  []securityaudit.Record
}

func (s *adminSecurityAuditStub) Save(_ context.Context, in securityaudit.SaveInput) (securityaudit.Record, error) {
	return securityaudit.Record{
		Category:   in.Category,
		Outcome:    in.Outcome,
		Source:     in.Source,
		Reason:     in.Reason,
		Path:       in.Path,
		HTTPStatus: in.HTTPStatus,
		RequestID:  in.RequestID,
		Actor:      in.Actor,
		ClientIP:   in.ClientIP,
	}, nil
}

func (s *adminSecurityAuditStub) ListRecent(_ context.Context, limit int) ([]securityaudit.Record, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if limit <= 0 || len(s.recent) == 0 {
		return []securityaudit.Record{}, nil
	}
	if limit > len(s.recent) {
		limit = len(s.recent)
	}
	out := make([]securityaudit.Record, limit)
	copy(out, s.recent[:limit])
	return out, nil
}

func (s *adminAuditStub) List(_ context.Context, q replayaudit.ListQuery) (replayaudit.ListResult, error) {
	if s.listErr != nil {
		return replayaudit.ListResult{}, s.listErr
	}
	s.lastQuery = q

	records := make([]replayaudit.Record, len(s.recent))
	copy(records, s.recent)
	if q.Limit > 0 && q.Limit < len(records) {
		records = records[:q.Limit]
	}
	return replayaudit.ListResult{
		Records: records,
		HasMore: q.Limit > 0 && len(s.recent) > len(records),
	}, nil
}

func TestAdminFailedEventsReturnsRecent(t *testing.T) {
	store := &adminStoreStub{
		recent: []failstore.FailedEvent{
			{EventID: "evt_2", Source: "github", Reason: "ERR_QUEUE_FULL", Body: json.RawMessage(`{"n":2}`)},
			{EventID: "evt_1", Source: "github", Reason: "ERR_PUBLISH_FAILED", Body: json.RawMessage(`{"n":1}`)},
		},
	}
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), store, &adminAuditStub{}, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodGet, "/admin/events/failed?limit=1", nil)
	rr := httptest.NewRecorder()
	h.FailedEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var payload struct {
		Enabled bool `json:"enabled"`
		Events  []struct {
			EventID string `json:"event_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !payload.Enabled {
		t.Fatal("expected enabled=true")
	}
	if len(payload.Events) != 1 || payload.Events[0].EventID != "evt_2" {
		t.Fatalf("unexpected events payload: %+v", payload.Events)
	}
}

func TestAdminReplayFailedEventDryRun(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Headers: map[string]string{"X-Test": "1"},
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
		},
	}
	pub := &adminPublisherStub{}
	h := NewAdminHandlerWithDependencies(config.Config{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), store, &adminAuditStub{}, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay", bytes.NewBufferString(`{"event_id":"evt_1","dry_run":true}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEvent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if pub.calls != 0 {
		t.Fatalf("expected no publish calls in dry-run, got %d", pub.calls)
	}
}

func TestAdminReplayFailedEventDryRunDoesNotRequirePublisher(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Headers: map[string]string{"X-Test": "1"},
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
		},
	}
	h := NewAdminHandlerWithDependencies(config.Config{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), store, &adminAuditStub{}, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay", bytes.NewBufferString(`{"event_id":"evt_1","dry_run":true}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEvent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestAdminReplayFailedEventPublishes(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Headers: map[string]string{"X-Test": "1"},
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
		},
	}
	pub := &adminPublisherStub{}
	audit := &adminAuditStub{}
	h := NewAdminHandlerWithDependencies(config.Config{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), store, audit, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay", bytes.NewBufferString(`{"event_id":"evt_1","header_overrides":{"X-Replay":"true"}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEvent(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	if pub.calls != 1 {
		t.Fatalf("expected one publish call, got %d", pub.calls)
	}
	if pub.last.headers["X-Replay"] != "true" {
		t.Fatalf("expected override header X-Replay=true, got %q", pub.last.headers["X-Replay"])
	}
	if audit.calls != 1 {
		t.Fatalf("expected one replay audit record, got %d", audit.calls)
	}
	if audit.saved[0].Result != "accepted" {
		t.Fatalf("expected replay audit result accepted, got %q", audit.saved[0].Result)
	}
}

func TestAdminReplayFailedEventNotFound(t *testing.T) {
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &adminStoreStub{}, &adminAuditStub{}, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay", bytes.NewBufferString(`{"event_id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEvent(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertAdminErrorCode(t, rr.Body.Bytes(), apperr.CodeReplayEventNotFound)
}

func TestAdminReplayFailedEventQueueFull(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
		},
	}
	pub := &adminPublisherStub{err: queue.ErrQueueFull}
	audit := &adminAuditStub{}
	h := NewAdminHandlerWithDependencies(config.Config{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), store, audit, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay", bytes.NewBufferString(`{"event_id":"evt_1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEvent(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	assertAdminErrorCode(t, rr.Body.Bytes(), apperr.CodeQueueFull)
	if audit.calls != 1 {
		t.Fatalf("expected one replay audit record, got %d", audit.calls)
	}
	if audit.saved[0].Result != "failed" || audit.saved[0].ErrorCode != string(apperr.CodeQueueFull) {
		t.Fatalf("unexpected replay audit failure payload: %+v", audit.saved[0])
	}
}

func TestAdminReplayFailedEventQueueThrottled(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
		},
	}
	pub := &adminPublisherStub{err: queue.ErrQueueThrottled}
	audit := &adminAuditStub{}
	h := NewAdminHandlerWithDependencies(config.Config{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), store, audit, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay", bytes.NewBufferString(`{"event_id":"evt_1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEvent(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	assertAdminErrorCode(t, rr.Body.Bytes(), apperr.CodeQueueThrottled)
	if audit.calls != 1 {
		t.Fatalf("expected one replay audit record, got %d", audit.calls)
	}
	if audit.saved[0].Result != "failed" || audit.saved[0].ErrorCode != string(apperr.CodeQueueThrottled) {
		t.Fatalf("unexpected replay audit failure payload: %+v", audit.saved[0])
	}
}

func TestAdminReplayFailedEventsBatchDryRun(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Headers: map[string]string{"X-Test": "1"},
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
			"evt_2": {
				EventID: "evt_2",
				Source:  "gitlab",
				Headers: map[string]string{"X-Test": "2"},
				Body:    json.RawMessage(`{"object_kind":"merge_request"}`),
			},
		},
	}
	pub := &adminPublisherStub{}
	audit := &adminAuditStub{}
	h := NewAdminHandlerWithDependencies(config.Config{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), store, audit, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay/batch", bytes.NewBufferString(`{"event_ids":["evt_1","evt_2"],"dry_run":true}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEventsBatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if pub.calls != 0 {
		t.Fatalf("expected no publish calls in dry-run batch, got %d", pub.calls)
	}
	var payload struct {
		Status  string `json:"status"`
		DryRun  bool   `json:"dry_run"`
		Summary struct {
			Requested int `json:"requested"`
			Validated int `json:"validated"`
			Failed    int `json:"failed"`
		} `json:"summary"`
		Results []struct {
			EventID string `json:"event_id"`
			Status  string `json:"status"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Status != "validated" || !payload.DryRun {
		t.Fatalf("unexpected batch dry-run payload: %+v", payload)
	}
	if payload.Summary.Requested != 2 || payload.Summary.Validated != 2 || payload.Summary.Failed != 0 {
		t.Fatalf("unexpected batch summary: %+v", payload.Summary)
	}
	if len(payload.Results) != 2 || payload.Results[0].Status != "validated" {
		t.Fatalf("unexpected batch results: %+v", payload.Results)
	}
	if audit.calls != 2 {
		t.Fatalf("expected two audit records, got %d", audit.calls)
	}
}

func TestAdminReplayFailedEventsBatchDryRunDoesNotRequirePublisher(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Headers: map[string]string{"X-Test": "1"},
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
		},
	}
	h := NewAdminHandlerWithDependencies(config.Config{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), store, &adminAuditStub{}, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay/batch", bytes.NewBufferString(`{"event_ids":["evt_1"],"dry_run":true}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEventsBatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestAdminReplayFailedEventsBatchPartialFailure(t *testing.T) {
	store := &adminStoreStub{
		events: map[string]failstore.FailedEvent{
			"evt_1": {
				EventID: "evt_1",
				Source:  "github",
				Headers: map[string]string{"X-Test": "1"},
				Body:    json.RawMessage(`{"action":"opened"}`),
			},
			"evt_2": {
				EventID: "evt_2",
				Source:  "gitlab",
				Headers: map[string]string{"X-Test": "2"},
				Body:    json.RawMessage(`{"object_kind":"merge_request"}`),
			},
		},
	}
	pub := &adminPublisherStub{}
	audit := &adminAuditStub{}
	h := NewAdminHandlerWithDependencies(config.Config{}, pub, slog.New(slog.NewTextHandler(io.Discard, nil)), store, audit, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay/batch", bytes.NewBufferString(`{"event_ids":["evt_1","missing","evt_2","evt_1"]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEventsBatch(rr, req)

	if rr.Code != http.StatusMultiStatus {
		t.Fatalf("expected 207, got %d", rr.Code)
	}
	if pub.calls != 2 {
		t.Fatalf("expected two publish calls for unique valid events, got %d", pub.calls)
	}
	var payload struct {
		Status  string `json:"status"`
		Summary struct {
			Requested int `json:"requested"`
			Accepted  int `json:"accepted"`
			Failed    int `json:"failed"`
			Published int `json:"published"`
		} `json:"summary"`
		Results []struct {
			EventID   string `json:"event_id"`
			Status    string `json:"status"`
			ErrorCode string `json:"error_code"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Status != "partial_failure" {
		t.Fatalf("expected partial_failure status, got %q", payload.Status)
	}
	if payload.Summary.Requested != 4 || payload.Summary.Accepted != 2 || payload.Summary.Failed != 1 || payload.Summary.Published != 2 {
		t.Fatalf("unexpected batch summary: %+v", payload.Summary)
	}
	if len(payload.Results) != 3 {
		t.Fatalf("expected 3 results after dedupe, got %d", len(payload.Results))
	}
	if payload.Results[1].EventID != "missing" || payload.Results[1].ErrorCode != string(apperr.CodeReplayEventNotFound) {
		t.Fatalf("unexpected failure result: %+v", payload.Results[1])
	}
	if audit.calls != 3 {
		t.Fatalf("expected three audit records, got %d", audit.calls)
	}
}

func TestAdminReplayFailedEventsBatchRejectsInvalidInput(t *testing.T) {
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &adminStoreStub{}, &adminAuditStub{}, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodPost, "/admin/events/replay/batch", bytes.NewBufferString(`{"event_ids":["   "]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ReplayFailedEventsBatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertAdminErrorCode(t, rr.Body.Bytes(), apperr.CodeReplayInputInvalid)
}

func TestAdminReplayAuditReturnsRecent(t *testing.T) {
	audit := &adminAuditStub{
		recent: []replayaudit.Record{
			{EventID: "evt_2", Actor: "dev2@example.com", Result: "accepted"},
			{EventID: "evt_1", Actor: "dev1@example.com", Result: "validated"},
		},
	}
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &adminStoreStub{}, audit, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodGet, "/admin/events/replay-audit?limit=2", nil)
	rr := httptest.NewRecorder()
	h.ReplayAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload struct {
		Enabled bool `json:"enabled"`
		Records []struct {
			EventID string `json:"event_id"`
			Actor   string `json:"actor"`
		} `json:"records"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !payload.Enabled {
		t.Fatal("expected enabled=true")
	}
	if len(payload.Records) != 2 || payload.Records[0].EventID != "evt_2" {
		t.Fatalf("unexpected replay audit records: %+v", payload.Records)
	}
	if audit.lastQuery.Limit != 2 {
		t.Fatalf("expected list query limit=2, got %d", audit.lastQuery.Limit)
	}
	if audit.lastQuery.Sort != replayaudit.SortDesc {
		t.Fatalf("expected default sort=desc, got %q", audit.lastQuery.Sort)
	}
}

func TestAdminReplayAuditParsesFiltersAndSort(t *testing.T) {
	audit := &adminAuditStub{
		recent: []replayaudit.Record{
			{EventID: "evt_2", Actor: "dev2@example.com", Result: "accepted"},
		},
	}
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &adminStoreStub{}, audit, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodGet, "/admin/events/replay-audit?limit=5&cursor=ra_cursor&actor=dev2@example.com&result=FAILED&event_id=evt_2&sort=asc", nil)
	rr := httptest.NewRecorder()
	h.ReplayAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if audit.lastQuery.Limit != 5 {
		t.Fatalf("expected limit=5, got %d", audit.lastQuery.Limit)
	}
	if audit.lastQuery.Cursor != "ra_cursor" {
		t.Fatalf("expected cursor=ra_cursor, got %q", audit.lastQuery.Cursor)
	}
	if audit.lastQuery.Actor != "dev2@example.com" {
		t.Fatalf("expected actor filter, got %q", audit.lastQuery.Actor)
	}
	if audit.lastQuery.Result != "failed" {
		t.Fatalf("expected normalized result filter failed, got %q", audit.lastQuery.Result)
	}
	if audit.lastQuery.EventID != "evt_2" {
		t.Fatalf("expected event_id=evt_2, got %q", audit.lastQuery.EventID)
	}
	if audit.lastQuery.Sort != replayaudit.SortAsc {
		t.Fatalf("expected sort=asc, got %q", audit.lastQuery.Sort)
	}
}

func TestAdminReplayAuditInvalidResultFilter(t *testing.T) {
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &adminStoreStub{}, &adminAuditStub{}, &adminSecurityAuditStub{})

	req := httptest.NewRequest(http.MethodGet, "/admin/events/replay-audit?result=unknown", nil)
	rr := httptest.NewRecorder()
	h.ReplayAudit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertAdminErrorCode(t, rr.Body.Bytes(), apperr.CodeReplayInputInvalid)
}

func TestAdminReplayAuditInvalidCursorReturnsBadRequest(t *testing.T) {
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &adminStoreStub{}, &adminAuditStub{
		listErr: replayaudit.ErrCursorNotFound,
	}, &adminSecurityAuditStub{})
	req := httptest.NewRequest(http.MethodGet, "/admin/events/replay-audit?cursor=missing_cursor", nil)
	rr := httptest.NewRecorder()
	h.ReplayAudit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertAdminErrorCode(t, rr.Body.Bytes(), apperr.CodeReplayInputInvalid)
}

func TestAdminSecurityAuditReturnsRecent(t *testing.T) {
	securityStore := &adminSecurityAuditStub{
		recent: []securityaudit.Record{
			{Source: "admin", Reason: "admin_jwt_invalid", Path: "/admin/configz", HTTPStatus: 401},
			{Source: "github", Reason: "webhook_auth_failed", Path: "/webhooks/github", HTTPStatus: 401},
		},
	}
	h := NewAdminHandlerWithDependencies(config.Config{}, &adminPublisherStub{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &adminStoreStub{}, &adminAuditStub{}, securityStore)

	req := httptest.NewRequest(http.MethodGet, "/admin/events/security-audit?limit=1", nil)
	rr := httptest.NewRecorder()
	h.SecurityAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var payload struct {
		Enabled bool `json:"enabled"`
		Records []struct {
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"records"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !payload.Enabled {
		t.Fatal("expected enabled=true")
	}
	if len(payload.Records) != 1 || payload.Records[0].Reason != "admin_jwt_invalid" {
		t.Fatalf("unexpected security audit records: %+v", payload.Records)
	}
}

func assertAdminErrorCode(t *testing.T, body []byte, expected apperr.Code) {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal error payload failed: %v body=%q", err, string(body))
	}
	if payload.Error.Code != string(expected) {
		t.Fatalf("expected error code %q, got %q", expected, payload.Error.Code)
	}
}
