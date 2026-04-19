package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/httpx"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
	"teampulsebridge/services/ingestion-gateway/internal/replayaudit"
)

const (
	adminFailedEventsDefaultLimit = 20
	adminFailedEventsMaxLimit     = 100
	adminReplayAuditDefaultLimit  = 20
	adminReplayAuditMaxLimit      = 100
	adminReplayRequestMaxBody     = 64 << 10
)

type AdminHandler struct {
	cfg       config.Config
	publisher queue.Publisher
	logger    *slog.Logger
	failed    failstore.Store
	audit     replayaudit.Store
}

func NewAdminHandler(cfg config.Config, publisher queue.Publisher, logger *slog.Logger) *AdminHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	var failedStore failstore.Store
	if cfg.FailedStoreEnabled {
		store, err := failstore.NewFileStore(cfg.FailedStorePath)
		if err != nil {
			logger.Error("admin failed-event explorer disabled due to invalid configuration",
				"path", cfg.FailedStorePath,
				"error", err,
				"error_code", apperr.CodeReplayConfigInvalid,
			)
		} else {
			failedStore = store
		}
	}

	var auditStore replayaudit.Store
	if cfg.ReplayAuditEnabled {
		store, err := replayaudit.NewFileStore(cfg.ReplayAuditPath)
		if err != nil {
			logger.Error("admin replay-audit history disabled due to invalid configuration",
				"path", cfg.ReplayAuditPath,
				"error", err,
				"error_code", apperr.CodeReplayConfigInvalid,
			)
		} else {
			auditStore = store
		}
	}

	return NewAdminHandlerWithDependencies(cfg, publisher, logger, failedStore, auditStore)
}

func NewAdminHandlerWithDependencies(cfg config.Config, publisher queue.Publisher, logger *slog.Logger, failed failstore.Store, audit replayaudit.Store) *AdminHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &AdminHandler{
		cfg:       cfg,
		publisher: publisher,
		logger:    logger,
		failed:    failed,
		audit:     audit,
	}
}

func (h *AdminHandler) Configz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":                    "ingestion-gateway",
		"queue_backend":              h.cfg.QueueBackend,
		"admin_auth_enabled":         h.cfg.AdminAuthEnabled,
		"request_timeout_sec":        h.cfg.RequestTimeoutSec,
		"queue_buffer":               h.cfg.QueueBuffer,
		"dedup_enabled":              h.cfg.DedupEnabled,
		"failed_event_store_enabled": h.cfg.FailedStoreEnabled,
		"failed_event_store_path":    h.cfg.FailedStorePath,
		"replay_audit_enabled":       h.cfg.ReplayAuditEnabled,
		"replay_audit_path":          h.cfg.ReplayAuditPath,
	})
}

func (h *AdminHandler) FailedEvents(w http.ResponseWriter, r *http.Request) {
	limit, err := parseFailedEventsLimit(r)
	if err != nil {
		h.respondError(w, r, http.StatusBadRequest, apperr.New(
			"handlers.admin.failedEvents",
			apperr.CodeReplayInputInvalid,
			"invalid failed-event list query",
			err,
		))
		return
	}

	if h.failed == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"events":  []any{},
		})
		return
	}

	events, err := h.failed.ListRecent(r.Context(), limit)
	if err != nil {
		h.respondError(w, r, http.StatusInternalServerError, apperr.New(
			"handlers.admin.failedEvents",
			apperr.CodeReplayReadFailed,
			"failed to load failed events",
			err,
		))
		return
	}

	summaries := make([]map[string]any, 0, len(events))
	for _, event := range events {
		summaries = append(summaries, map[string]any{
			"event_id":     event.EventID,
			"source":       event.Source,
			"reason":       event.Reason,
			"failed_at":    event.FailedAt,
			"payload_hash": event.PayloadHash,
			"body_bytes":   len(event.Body),
			"body_preview": bodyPreview(event.Body, 220),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"events":  summaries,
	})
}

type replayFailedEventRequest struct {
	EventID         string            `json:"event_id"`
	DryRun          bool              `json:"dry_run"`
	HeaderOverrides map[string]string `json:"header_overrides"`
}

func (h *AdminHandler) ReplayFailedEvent(w http.ResponseWriter, r *http.Request) {
	actor := replayActorFromRequest(r)
	requestID := httpx.RequestIDFromContext(r.Context())

	if h.failed == nil {
		h.respondError(w, r, http.StatusServiceUnavailable, apperr.New(
			"handlers.admin.replayFailedEvent",
			apperr.CodeReplayConfigInvalid,
			"failed-event store is not enabled",
			nil,
		))
		return
	}
	if h.publisher == nil {
		h.respondError(w, r, http.StatusServiceUnavailable, apperr.New(
			"handlers.admin.replayFailedEvent",
			apperr.CodeReplayConfigInvalid,
			"publisher is not configured",
			nil,
		))
		return
	}

	var req replayFailedEventRequest
	if err := decodeReplayFailedEventRequest(w, r, &req); err != nil {
		h.respondError(w, r, http.StatusBadRequest, apperr.New(
			"handlers.admin.replayFailedEvent",
			apperr.CodeReplayInputInvalid,
			"invalid replay request payload",
			err,
		))
		return
	}
	req.EventID = strings.TrimSpace(req.EventID)
	if req.EventID == "" {
		h.respondError(w, r, http.StatusBadRequest, apperr.New(
			"handlers.admin.replayFailedEvent",
			apperr.CodeReplayInputInvalid,
			"event_id is required",
			nil,
		))
		return
	}
	mode := replayMode(req.DryRun)

	record, err := h.failed.GetByID(r.Context(), req.EventID)
	if errors.Is(err, failstore.ErrNotFound) {
		h.recordReplayAudit(r, replayaudit.SaveInput{
			EventID:    req.EventID,
			Actor:      actor,
			Mode:       mode,
			Result:     "failed",
			ErrorCode:  string(apperr.CodeReplayEventNotFound),
			HTTPStatus: http.StatusNotFound,
			RequestID:  requestID,
		})
		h.respondError(w, r, http.StatusNotFound, apperr.New(
			"handlers.admin.replayFailedEvent",
			apperr.CodeReplayEventNotFound,
			"failed event not found",
			err,
		))
		return
	}
	if err != nil {
		h.recordReplayAudit(r, replayaudit.SaveInput{
			EventID:    req.EventID,
			Actor:      actor,
			Mode:       mode,
			Result:     "failed",
			ErrorCode:  string(apperr.CodeReplayReadFailed),
			HTTPStatus: http.StatusInternalServerError,
			RequestID:  requestID,
		})
		h.respondError(w, r, http.StatusInternalServerError, apperr.New(
			"handlers.admin.replayFailedEvent",
			apperr.CodeReplayReadFailed,
			"failed to load failed event",
			err,
		))
		return
	}

	headers := cloneReplayHeaders(record.Headers, req.HeaderOverrides)
	if req.DryRun {
		h.recordReplayAudit(r, replayaudit.SaveInput{
			EventID:    record.EventID,
			Source:     record.Source,
			Actor:      actor,
			Mode:       mode,
			Result:     "validated",
			HTTPStatus: http.StatusOK,
			RequestID:  requestID,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "validated",
			"event_id":  record.EventID,
			"source":    record.Source,
			"dry_run":   true,
			"published": false,
			"bytes":     len(record.Body),
		})
		return
	}

	if err := h.publisher.Publish(r.Context(), record.Source, record.Body, headers); err != nil {
		status := http.StatusInternalServerError
		code := apperr.CodeReplayPublishFailed
		if errors.Is(err, queue.ErrQueueFull) {
			status = http.StatusServiceUnavailable
			code = apperr.CodeQueueFull
		}
		h.recordReplayAudit(r, replayaudit.SaveInput{
			EventID:    record.EventID,
			Source:     record.Source,
			Actor:      actor,
			Mode:       mode,
			Result:     "failed",
			ErrorCode:  string(code),
			HTTPStatus: status,
			RequestID:  requestID,
		})
		h.respondError(w, r, status, apperr.New(
			"handlers.admin.replayFailedEvent",
			code,
			"failed to publish replay event",
			err,
		))
		return
	}
	h.recordReplayAudit(r, replayaudit.SaveInput{
		EventID:    record.EventID,
		Source:     record.Source,
		Actor:      actor,
		Mode:       mode,
		Result:     "accepted",
		HTTPStatus: http.StatusAccepted,
		RequestID:  requestID,
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":    "accepted",
		"event_id":  record.EventID,
		"source":    record.Source,
		"dry_run":   false,
		"published": true,
		"bytes":     len(record.Body),
	})
}

func (h *AdminHandler) ReplayAudit(w http.ResponseWriter, r *http.Request) {
	query, err := parseReplayAuditQuery(r)
	if err != nil {
		h.respondError(w, r, http.StatusBadRequest, apperr.New(
			"handlers.admin.replayAudit",
			apperr.CodeReplayInputInvalid,
			"invalid replay-audit list query",
			err,
		))
		return
	}
	if h.audit == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"records": []any{},
			"page": map[string]any{
				"has_more":    false,
				"next_cursor": "",
			},
		})
		return
	}

	list, err := h.audit.List(r.Context(), query)
	if err != nil {
		status := http.StatusInternalServerError
		code := apperr.CodeReplayReadFailed
		message := "failed to load replay audit history"
		if errors.Is(err, replayaudit.ErrInvalidListQuery) || errors.Is(err, replayaudit.ErrCursorNotFound) {
			status = http.StatusBadRequest
			code = apperr.CodeReplayInputInvalid
			message = "invalid replay-audit list query"
		}
		h.respondError(w, r, status, apperr.New(
			"handlers.admin.replayAudit",
			code,
			message,
			err,
		))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"records": list.Records,
		"page": map[string]any{
			"has_more":    list.HasMore,
			"next_cursor": list.NextCursor,
		},
	})
}

func (h *AdminHandler) respondError(w http.ResponseWriter, r *http.Request, status int, err *apperr.Error) {
	httpx.WriteError(w, r.Context(), status, err, nil)
}

func parseFailedEventsLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return adminFailedEventsDefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}
	if n < 1 || n > adminFailedEventsMaxLimit {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return n, nil
}

func parseReplayAuditLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return adminReplayAuditDefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}
	if n < 1 || n > adminReplayAuditMaxLimit {
		return 0, errors.New("limit must be between 1 and 100")
	}
	return n, nil
}

func parseReplayAuditQuery(r *http.Request) (replayaudit.ListQuery, error) {
	limit, err := parseReplayAuditLimit(r)
	if err != nil {
		return replayaudit.ListQuery{}, err
	}

	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	actor := strings.TrimSpace(r.URL.Query().Get("actor"))
	result := strings.TrimSpace(r.URL.Query().Get("result"))
	eventID := strings.TrimSpace(r.URL.Query().Get("event_id"))
	sortOrder, err := replayaudit.ParseSortOrder(r.URL.Query().Get("sort"))
	if err != nil {
		return replayaudit.ListQuery{}, err
	}

	if len(cursor) > 128 {
		return replayaudit.ListQuery{}, errors.New("cursor must be <= 128 characters")
	}
	if len(actor) > 256 {
		return replayaudit.ListQuery{}, errors.New("actor must be <= 256 characters")
	}
	if len(eventID) > 256 {
		return replayaudit.ListQuery{}, errors.New("event_id must be <= 256 characters")
	}
	if result != "" {
		normalizedResult := strings.ToLower(result)
		switch normalizedResult {
		case "accepted", "validated", "failed":
			result = normalizedResult
		default:
			return replayaudit.ListQuery{}, errors.New("result must be one of accepted|validated|failed")
		}
	}

	return replayaudit.ListQuery{
		Limit:   limit,
		Cursor:  cursor,
		Actor:   actor,
		Result:  result,
		EventID: eventID,
		Sort:    sortOrder,
	}, nil
}

func bodyPreview(body []byte, limit int) string {
	if len(body) == 0 || limit <= 0 {
		return ""
	}
	clean := strings.TrimSpace(string(body))
	if len(clean) <= limit {
		return clean
	}
	return clean[:limit] + "...truncated"
}

func decodeReplayFailedEventRequest(w http.ResponseWriter, r *http.Request, out *replayFailedEventRequest) error {
	r.Body = http.MaxBytesReader(w, r.Body, adminReplayRequestMaxBody)
	defer func() {
		_ = r.Body.Close()
	}()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func cloneReplayHeaders(base map[string]string, overrides map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}

func (h *AdminHandler) recordReplayAudit(r *http.Request, in replayaudit.SaveInput) {
	if h.audit == nil {
		return
	}
	if _, err := h.audit.Save(r.Context(), in); err != nil {
		h.logger.Error("failed to persist replay audit record",
			"event_id", in.EventID,
			"actor", in.Actor,
			"mode", in.Mode,
			"result", in.Result,
			"error", err,
			"error_code", apperr.CodeReplayReadFailed,
		)
	}
}

func replayMode(dryRun bool) string {
	if dryRun {
		return "dry_run"
	}
	return "publish"
}

func replayActorFromRequest(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Operator")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-User")); v != "" {
		return v
	}

	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		token := strings.TrimSpace(authz[len("Bearer "):])
		if token != "" {
			claims := jwt.MapClaims{}
			parser := jwt.NewParser()
			if _, _, err := parser.ParseUnverified(token, claims); err == nil {
				for _, key := range []string{"email", "sub", "preferred_username", "name"} {
					if value := claimString(claims[key]); value != "" {
						return value
					}
				}
			}
		}
	}

	ip := clientIP(r.RemoteAddr)
	if ip == "" {
		return "unknown"
	}
	return "ip:" + ip
}

func claimString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}
