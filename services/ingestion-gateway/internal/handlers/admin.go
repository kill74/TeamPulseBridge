package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"teampulsebridge/services/ingestion-gateway/internal/securityaudit"
)

const (
	adminFailedEventsDefaultLimit = 20
	adminFailedEventsMaxLimit     = 100
	adminReplayAuditDefaultLimit  = 20
	adminReplayAuditMaxLimit      = 100
	adminReplayBatchMaxEvents     = 25
	adminReplayRequestMaxBody     = 64 << 10
)

type AdminHandler struct {
	cfg       config.Config
	publisher queue.Publisher
	logger    *slog.Logger
	failed    failstore.Store
	audit     replayaudit.Store
	security  securityaudit.Store
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

	var securityStore securityaudit.Store
	if cfg.SecurityAuditEnabled {
		store, err := securityaudit.NewFileStore(cfg.SecurityAuditPath, cfg.SecurityAuditRetentionDays)
		if err != nil {
			logger.Error("admin security audit explorer disabled due to invalid configuration",
				"path", cfg.SecurityAuditPath,
				"retention_days", cfg.SecurityAuditRetentionDays,
				"error", err,
				"error_code", apperr.CodeReplayConfigInvalid,
			)
		} else {
			securityStore = store
		}
	}

	return NewAdminHandlerWithDependencies(cfg, publisher, logger, failedStore, auditStore, securityStore)
}

func NewAdminHandlerWithDependencies(cfg config.Config, publisher queue.Publisher, logger *slog.Logger, failed failstore.Store, audit replayaudit.Store, security securityaudit.Store) *AdminHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &AdminHandler{
		cfg:       cfg,
		publisher: publisher,
		logger:    logger,
		failed:    failed,
		audit:     audit,
		security:  security,
	}
}

func (h *AdminHandler) Configz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":                               "ingestion-gateway",
		"queue_backend":                         h.cfg.QueueBackend,
		"admin_auth_enabled":                    h.cfg.AdminAuthEnabled,
		"request_timeout_sec":                   h.cfg.RequestTimeoutSec,
		"queue_buffer":                          h.cfg.QueueBuffer,
		"queue_backpressure_enabled":            h.cfg.QueueBackpressureEnabled,
		"queue_backpressure_soft_limit_percent": h.cfg.QueueBackpressureSoftLimitPercent,
		"queue_backpressure_hard_limit_percent": h.cfg.QueueBackpressureHardLimitPercent,
		"queue_failure_budget_percent":          h.cfg.QueueFailureBudgetPercent,
		"queue_failure_budget_window":           h.cfg.QueueFailureBudgetWindow,
		"queue_failure_budget_min_samples":      h.cfg.QueueFailureBudgetMinSamples,
		"queue_throttle_retry_after_sec":        h.cfg.QueueThrottleRetryAfterSec,
		"dedup_enabled":                         h.cfg.DedupEnabled,
		"failed_event_store_enabled":            h.cfg.FailedStoreEnabled,
		"failed_event_store_path":               h.cfg.FailedStorePath,
		"replay_audit_enabled":                  h.cfg.ReplayAuditEnabled,
		"replay_audit_path":                     h.cfg.ReplayAuditPath,
		"security_audit_enabled":                h.cfg.SecurityAuditEnabled,
		"security_audit_path":                   h.cfg.SecurityAuditPath,
		"security_audit_retention_days":         h.cfg.SecurityAuditRetentionDays,
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

type replayFailedEventsBatchRequest struct {
	EventIDs        []string          `json:"event_ids"`
	DryRun          bool              `json:"dry_run"`
	HeaderOverrides map[string]string `json:"header_overrides"`
}

type replayExecutionInput struct {
	EventID         string
	DryRun          bool
	HeaderOverrides map[string]string
	Actor           string
	RequestID       string
}

type replayExecutionResult struct {
	EventID    string `json:"event_id"`
	Source     string `json:"source,omitempty"`
	Status     string `json:"status"`
	HTTPStatus int    `json:"http_status"`
	Published  bool   `json:"published"`
	Bytes      int    `json:"bytes"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

type replayBatchSummary struct {
	Requested int `json:"requested"`
	Processed int `json:"processed"`
	Succeeded int `json:"succeeded"`
	Accepted  int `json:"accepted"`
	Validated int `json:"validated"`
	Failed    int `json:"failed"`
	Published int `json:"published"`
}

func (h *AdminHandler) ReplayFailedEvent(w http.ResponseWriter, r *http.Request) {
	actor := replayActorFromRequest(r)
	requestID := httpx.RequestIDFromContext(r.Context())

	if status, replayErr := h.replayStoreDependencyError("handlers.admin.replayFailedEvent"); replayErr != nil {
		h.respondError(w, r, status, replayErr)
		return
	}

	var req replayFailedEventRequest
	if err := decodeAdminJSONRequest(w, r, adminReplayRequestMaxBody, &req); err != nil {
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
	if !req.DryRun {
		if status, replayErr := h.replayPublisherDependencyError("handlers.admin.replayFailedEvent"); replayErr != nil {
			h.respondError(w, r, status, replayErr)
			return
		}
	}

	result, replayErr := h.executeReplay(r.Context(), replayExecutionInput{
		EventID:         req.EventID,
		DryRun:          req.DryRun,
		HeaderOverrides: req.HeaderOverrides,
		Actor:           actor,
		RequestID:       requestID,
	})
	if replayErr != nil {
		h.respondError(w, r, result.HTTPStatus, replayErr)
		return
	}

	writeJSON(w, result.HTTPStatus, map[string]any{
		"status":    result.Status,
		"event_id":  result.EventID,
		"source":    result.Source,
		"dry_run":   req.DryRun,
		"published": result.Published,
		"bytes":     result.Bytes,
	})
}

func (h *AdminHandler) ReplayFailedEventsBatch(w http.ResponseWriter, r *http.Request) {
	if status, replayErr := h.replayStoreDependencyError("handlers.admin.replayFailedEventsBatch"); replayErr != nil {
		h.respondError(w, r, status, replayErr)
		return
	}

	var req replayFailedEventsBatchRequest
	if err := decodeAdminJSONRequest(w, r, adminReplayRequestMaxBody, &req); err != nil {
		h.respondError(w, r, http.StatusBadRequest, apperr.New(
			"handlers.admin.replayFailedEventsBatch",
			apperr.CodeReplayInputInvalid,
			"invalid replay batch request payload",
			err,
		))
		return
	}

	eventIDs, err := normalizeReplayEventIDs(req.EventIDs)
	if err != nil {
		h.respondError(w, r, http.StatusBadRequest, apperr.New(
			"handlers.admin.replayFailedEventsBatch",
			apperr.CodeReplayInputInvalid,
			"invalid replay batch request payload",
			err,
		))
		return
	}
	if !req.DryRun {
		if status, replayErr := h.replayPublisherDependencyError("handlers.admin.replayFailedEventsBatch"); replayErr != nil {
			h.respondError(w, r, status, replayErr)
			return
		}
	}

	actor := replayActorFromRequest(r)
	requestID := httpx.RequestIDFromContext(r.Context())
	results := make([]replayExecutionResult, 0, len(eventIDs))
	summary := replayBatchSummary{
		Requested: len(req.EventIDs),
	}
	for _, eventID := range eventIDs {
		result, _ := h.executeReplay(r.Context(), replayExecutionInput{
			EventID:         eventID,
			DryRun:          req.DryRun,
			HeaderOverrides: req.HeaderOverrides,
			Actor:           actor,
			RequestID:       requestID,
		})
		results = append(results, result)
		summary.Processed++
		switch result.Status {
		case "validated":
			summary.Validated++
			summary.Succeeded++
		case "accepted":
			summary.Accepted++
			summary.Published++
			summary.Succeeded++
		default:
			summary.Failed++
		}
	}

	writeJSON(w, batchReplayHTTPStatus(req.DryRun, summary), map[string]any{
		"status":  batchReplayStatus(summary),
		"dry_run": req.DryRun,
		"summary": summary,
		"results": results,
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

func (h *AdminHandler) SecurityAudit(w http.ResponseWriter, r *http.Request) {
	limit, err := parseSecurityAuditLimit(r)
	if err != nil {
		h.respondError(w, r, http.StatusBadRequest, apperr.New(
			"handlers.admin.securityAudit",
			apperr.CodeReplayInputInvalid,
			"invalid security-audit list query",
			err,
		))
		return
	}
	if h.security == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"records": []any{},
		})
		return
	}

	records, err := h.security.ListRecent(r.Context(), limit)
	if err != nil {
		h.respondError(w, r, http.StatusInternalServerError, apperr.New(
			"handlers.admin.securityAudit",
			apperr.CodeReplayReadFailed,
			"failed to load security audit history",
			err,
		))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"records": records,
	})
}

func (h *AdminHandler) respondError(w http.ResponseWriter, r *http.Request, status int, err *apperr.Error) {
	httpx.WriteError(w, r.Context(), status, err, nil)
}

func parseFailedEventsLimit(r *http.Request) (int, error) {
	return parseAdminLimit(r.URL.Query().Get("limit"), adminFailedEventsDefaultLimit, adminFailedEventsMaxLimit)
}

func parseReplayAuditLimit(r *http.Request) (int, error) {
	return parseAdminLimit(r.URL.Query().Get("limit"), adminReplayAuditDefaultLimit, adminReplayAuditMaxLimit)
}

func parseSecurityAuditLimit(r *http.Request) (int, error) {
	return parseAdminLimit(r.URL.Query().Get("limit"), adminReplayAuditDefaultLimit, adminReplayAuditMaxLimit)
}

func parseAdminLimit(raw string, defaultLimit, maxLimit int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}
	if n < 1 || n > maxLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maxLimit)
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

func normalizeReplayEventIDs(rawIDs []string) ([]string, error) {
	if len(rawIDs) == 0 {
		return nil, errors.New("event_ids must contain at least one event id")
	}

	seen := make(map[string]struct{}, len(rawIDs))
	ids := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		eventID := strings.TrimSpace(rawID)
		if eventID == "" {
			return nil, errors.New("event_ids must not contain empty values")
		}
		if len(eventID) > 256 {
			return nil, errors.New("event_ids must contain values <= 256 characters")
		}
		if _, ok := seen[eventID]; ok {
			continue
		}
		seen[eventID] = struct{}{}
		ids = append(ids, eventID)
	}
	if len(ids) == 0 {
		return nil, errors.New("event_ids must contain at least one event id")
	}
	if len(ids) > adminReplayBatchMaxEvents {
		return nil, errors.New("event_ids must contain at most 25 unique event ids")
	}
	return ids, nil
}

func batchReplayStatus(summary replayBatchSummary) string {
	switch {
	case summary.Failed == 0 && summary.Accepted > 0:
		return "accepted"
	case summary.Failed == 0 && summary.Validated > 0:
		return "validated"
	case summary.Failed == summary.Processed:
		return "failed"
	default:
		return "partial_failure"
	}
}

func batchReplayHTTPStatus(dryRun bool, summary replayBatchSummary) int {
	if summary.Failed > 0 {
		return http.StatusMultiStatus
	}
	if dryRun {
		return http.StatusOK
	}
	return http.StatusAccepted
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

func decodeAdminJSONRequest(w http.ResponseWriter, r *http.Request, maxBytes int64, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	defer func() {
		_ = r.Body.Close()
	}()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain a single JSON object")
		}
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

func (h *AdminHandler) replayStoreDependencyError(op string) (int, *apperr.Error) {
	if h.failed == nil {
		return http.StatusServiceUnavailable, apperr.New(
			op,
			apperr.CodeReplayConfigInvalid,
			"failed-event store is not enabled",
			nil,
		)
	}
	return 0, nil
}

func (h *AdminHandler) replayPublisherDependencyError(op string) (int, *apperr.Error) {
	if h.publisher == nil {
		return http.StatusServiceUnavailable, apperr.New(
			op,
			apperr.CodeReplayConfigInvalid,
			"publisher is not configured",
			nil,
		)
	}
	return 0, nil
}

func (h *AdminHandler) executeReplay(ctx context.Context, in replayExecutionInput) (replayExecutionResult, *apperr.Error) {
	eventID := strings.TrimSpace(in.EventID)
	mode := replayMode(in.DryRun)
	result := replayExecutionResult{
		EventID: eventID,
		Status:  "failed",
	}

	record, err := h.failed.GetByID(ctx, eventID)
	if errors.Is(err, failstore.ErrNotFound) {
		result.HTTPStatus = http.StatusNotFound
		result.ErrorCode = string(apperr.CodeReplayEventNotFound)
		result.Message = "failed event not found"
		h.recordReplayAuditFromContext(ctx, replayaudit.SaveInput{
			EventID:    eventID,
			Actor:      in.Actor,
			Mode:       mode,
			Result:     "failed",
			ErrorCode:  result.ErrorCode,
			HTTPStatus: result.HTTPStatus,
			RequestID:  in.RequestID,
		})
		return result, apperr.New(
			"handlers.admin.executeReplay",
			apperr.CodeReplayEventNotFound,
			result.Message,
			err,
		)
	}
	if err != nil {
		result.HTTPStatus = http.StatusInternalServerError
		result.ErrorCode = string(apperr.CodeReplayReadFailed)
		result.Message = "failed to load failed event"
		h.recordReplayAuditFromContext(ctx, replayaudit.SaveInput{
			EventID:    eventID,
			Actor:      in.Actor,
			Mode:       mode,
			Result:     "failed",
			ErrorCode:  result.ErrorCode,
			HTTPStatus: result.HTTPStatus,
			RequestID:  in.RequestID,
		})
		return result, apperr.New(
			"handlers.admin.executeReplay",
			apperr.CodeReplayReadFailed,
			result.Message,
			err,
		)
	}

	result.EventID = record.EventID
	result.Source = record.Source
	result.Bytes = len(record.Body)

	headers := cloneReplayHeaders(record.Headers, in.HeaderOverrides)
	if in.DryRun {
		result.Status = "validated"
		result.HTTPStatus = http.StatusOK
		h.recordReplayAuditFromContext(ctx, replayaudit.SaveInput{
			EventID:    record.EventID,
			Source:     record.Source,
			Actor:      in.Actor,
			Mode:       mode,
			Result:     "validated",
			HTTPStatus: result.HTTPStatus,
			RequestID:  in.RequestID,
		})
		return result, nil
	}

	if h.publisher == nil {
		result.HTTPStatus = http.StatusServiceUnavailable
		result.ErrorCode = string(apperr.CodeReplayConfigInvalid)
		result.Message = "publisher is not configured"
		h.recordReplayAuditFromContext(ctx, replayaudit.SaveInput{
			EventID:    record.EventID,
			Source:     record.Source,
			Actor:      in.Actor,
			Mode:       mode,
			Result:     "failed",
			ErrorCode:  result.ErrorCode,
			HTTPStatus: result.HTTPStatus,
			RequestID:  in.RequestID,
		})
		return result, apperr.New(
			"handlers.admin.executeReplay",
			apperr.CodeReplayConfigInvalid,
			result.Message,
			nil,
		)
	}

	if err := h.publisher.Publish(ctx, record.Source, record.Body, headers); err != nil {
		code := apperr.CodeReplayPublishFailed
		result.HTTPStatus = http.StatusInternalServerError
		if errors.Is(err, queue.ErrQueueThrottled) {
			code = apperr.CodeQueueThrottled
			result.HTTPStatus = http.StatusTooManyRequests
		} else if errors.Is(err, queue.ErrQueueFull) {
			code = apperr.CodeQueueFull
			result.HTTPStatus = http.StatusServiceUnavailable
		}
		result.ErrorCode = string(code)
		result.Message = "failed to publish replay event"
		h.recordReplayAuditFromContext(ctx, replayaudit.SaveInput{
			EventID:    record.EventID,
			Source:     record.Source,
			Actor:      in.Actor,
			Mode:       mode,
			Result:     "failed",
			ErrorCode:  result.ErrorCode,
			HTTPStatus: result.HTTPStatus,
			RequestID:  in.RequestID,
		})
		return result, apperr.New(
			"handlers.admin.executeReplay",
			code,
			result.Message,
			err,
		)
	}

	result.Status = "accepted"
	result.Published = true
	result.HTTPStatus = http.StatusAccepted
	h.recordReplayAuditFromContext(ctx, replayaudit.SaveInput{
		EventID:    record.EventID,
		Source:     record.Source,
		Actor:      in.Actor,
		Mode:       mode,
		Result:     "accepted",
		HTTPStatus: result.HTTPStatus,
		RequestID:  in.RequestID,
	})
	return result, nil
}

func (h *AdminHandler) recordReplayAuditFromContext(ctx context.Context, in replayaudit.SaveInput) {
	if h.audit == nil {
		return
	}
	if _, err := h.audit.Save(ctx, in); err != nil {
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
