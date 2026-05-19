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
	"time"
	"unicode/utf8"

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
	adminReplayRequestMaxBody     = 256 << 10
)

type AdminHandler struct {
	cfg       config.Config
	publisher queue.Publisher
	logger    *slog.Logger
	failed    failstore.Store
	audit     replayaudit.Store
	security  securityaudit.Store
	flags     *config.FeatureFlags
}

func NewAdminHandler(cfg config.Config, publisher queue.Publisher, logger *slog.Logger) *AdminHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return NewAdminHandlerWithDependencies(cfg, publisher, logger, nil, nil, nil, nil)
}

func NewAdminHandlerWithDependencies(cfg config.Config, publisher queue.Publisher, logger *slog.Logger, failed failstore.Store, audit replayaudit.Store, security securityaudit.Store, flags *config.FeatureFlags) *AdminHandler {
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
		flags:     flags,
	}
}

func (h *AdminHandler) Configz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":                               "ingestion-gateway",
		"queue_backend":                         h.cfg.QueueBackend,
		"admin_auth_enabled":                    h.cfg.AdminAuthEnabled,
		"request_timeout_sec":                   h.cfg.RequestTimeoutSec,
		"queue_buffer":                          h.cfg.QueueBuffer,
		"queue_workers":                         h.cfg.QueueWorkers,
		"queue_bulkhead_enabled":                h.cfg.QueueBulkheadEnabled,
		"queue_bulkhead_buffer_per_source":      h.cfg.QueueBulkheadBufferPerSource,
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
		"source_rate_limit_enabled":             h.cfg.SourceRateLimitEnabled,
		"source_rate_limit_default":             h.cfg.SourceRateLimitDefault,
		"rate_limit_backend":                    h.cfg.RateLimitBackend,
		"pubsub_publish_goroutines":             h.cfg.PubSubPublishGoroutines,
		"pubsub_max_outstanding_messages":       h.cfg.PubSubMaxOutstandingMessages,
		"pubsub_max_outstanding_bytes":          h.cfg.PubSubMaxOutstandingBytes,
		"pubsub_flow_control_behavior":          h.cfg.PubSubFlowControlBehavior,
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
	actor := replayActorFromRequest(r, h.cfg.AdminJWTSecret)
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

	actor := replayActorFromRequest(r, h.cfg.AdminJWTSecret)
	requestID := httpx.RequestIDFromContext(r.Context())
	results := make([]replayExecutionResult, 0, len(eventIDs))
	summary := replayBatchSummary{
		Requested: len(req.EventIDs),
	}
	for _, eventID := range eventIDs {
		select {
		case <-r.Context().Done():
			h.logger.Warn("batch replay cancelled",
				"processed", summary.Processed,
				"remaining", len(eventIDs)-summary.Processed,
			)
			goto writeResponse
		default:
		}

		result, err := h.executeReplay(r.Context(), replayExecutionInput{
			EventID:         eventID,
			DryRun:          req.DryRun,
			HeaderOverrides: req.HeaderOverrides,
			Actor:           actor,
			RequestID:       requestID,
		})
		if err != nil {
			h.logger.Warn("batch replay event failed",
				"event_id", eventID,
				"error", err,
			)
		}
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

writeResponse:
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

type featureFlagUpdateRequest struct {
	Flag    string `json:"flag"`
	Enabled bool   `json:"enabled"`
}

func (h *AdminHandler) FeatureFlags(w http.ResponseWriter, r *http.Request) {
	if h.flags == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"flags":   map[string]bool{},
		})
		return
	}

	if r.Method == http.MethodPost {
		var req featureFlagUpdateRequest
		if err := decodeAdminJSONRequest(w, r, 1<<10, &req); err != nil {
			h.respondError(w, r, http.StatusBadRequest, apperr.New(
				"handlers.admin.featureFlags",
				apperr.CodeReplayInputInvalid,
				"invalid feature flag update payload",
				err,
			))
			return
		}

		req.Flag = strings.TrimSpace(req.Flag)
		if req.Flag == "" {
			h.respondError(w, r, http.StatusBadRequest, apperr.New(
				"handlers.admin.featureFlags",
				apperr.CodeReplayInputInvalid,
				"flag name is required",
				nil,
			))
			return
		}

		if !h.flags.Set(req.Flag, req.Enabled) {
			h.respondError(w, r, http.StatusNotFound, apperr.New(
				"handlers.admin.featureFlags",
				apperr.CodeReplayInputInvalid,
				fmt.Sprintf("feature flag %q not found", req.Flag),
				nil,
			))
			return
		}

		actor := replayActorFromRequest(r, h.cfg.AdminJWTSecret)
		h.logger.Info("feature flag updated", "flag", req.Flag, "enabled", req.Enabled, "actor", actor)
		if h.security != nil {
			_, _ = h.security.Save(r.Context(), securityaudit.SaveInput{
				Category:   "feature_flag_change",
				Outcome:    "accepted",
				Source:     "admin",
				Reason:     "flag:" + req.Flag + "=" + strconv.FormatBool(req.Enabled),
				Path:       r.URL.Path,
				HTTPStatus: http.StatusOK,
				RequestID:  httpx.RequestIDFromContext(r.Context()),
				Actor:      actor,
				ClientIP:   httpx.ClientIP(r, h.cfg.TrustedProxyCIDRs),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"flag":    req.Flag,
			"enabled": req.Enabled,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"flags":   h.flags.List(),
	})
}

func (h *AdminHandler) respondError(w http.ResponseWriter, r *http.Request, status int, err *apperr.Error) {
	httpx.WriteError(w, r.Context(), status, err, nil)
	_ = h
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
	clean = redactSecrets(clean)
	if len(clean) <= limit {
		return clean
	}
	truncated := clean[:limit]
	for len(truncated) > 0 {
		if _, size := utf8.DecodeLastRuneInString(truncated); size == 0 || size > len(truncated) {
			truncated = truncated[:len(truncated)-1]
		} else {
			break
		}
	}
	return truncated + "...truncated"
}

func redactSecrets(s string) string {
	type replacement struct {
		start int
		end   int
	}
	var replacements []replacement
	lower := strings.ToLower(s)
	patterns := []string{
		`"token"`, `"secret"`, `"password"`, `"key"`, `"authorization"`,
		`"api_key"`, `"api-key"`, `"apikey"`, `"access_token"`, `"access-token"`,
		`"client_secret"`, `"client-secret"`, `"bearer"`, `"credential"`,
		`"auth_token"`, `"auth-token"`, `"private_key"`, `"private-key"`,
	}
	for _, pattern := range patterns {
		searchStart := 0
		for {
			idx := strings.Index(lower[searchStart:], pattern)
			if idx == -1 {
				break
			}
			idx += searchStart
			colonIdx := strings.Index(s[idx:], ":")
			if colonIdx == -1 {
				searchStart = idx + len(pattern)
				continue
			}
			start := idx + colonIdx + 1
			end := start
			for end < len(s) && s[end] != ',' && s[end] != '}' && s[end] != '\n' {
				end++
			}
			if end > start {
				replacements = append(replacements, replacement{start, end})
			}
			searchStart = idx + len(pattern)
		}
	}
	if len(replacements) == 0 {
		return s
	}
	var b strings.Builder
	prev := 0
	for _, r := range replacements {
		b.WriteString(s[prev:r.start])
		b.WriteString("[REDACTED]")
		prev = r.end
	}
	b.WriteString(s[prev:])
	return b.String()
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

var replayHeaderDenylist = map[string]struct{}{
	"authorization":  {},
	"cookie":         {},
	"x-event-id":     {},
	"traceparent":    {},
	"x-operator":     {},
	"x-user":         {},
	"x-replay-source": {},
	"x-replay-event-id": {},
	"x-replay-timestamp": {},
	"x-replay-request-id": {},
	"x-replay-actor": {},
}

func cloneReplayHeaders(base, overrides map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if _, denied := replayHeaderDenylist[strings.ToLower(key)]; denied {
			return nil, fmt.Errorf("header %q is not allowed in replay overrides", key)
		}
		out[key] = strings.TrimSpace(v)
	}
	return out, nil
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

	headers, err := cloneReplayHeaders(record.Headers, in.HeaderOverrides)
	if err != nil {
		result.HTTPStatus = http.StatusBadRequest
		result.ErrorCode = string(apperr.CodeReplayInputInvalid)
		result.Message = err.Error()
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
			apperr.CodeReplayInputInvalid,
			result.Message,
			err,
		)
	}
	headers["X-Replay-Source"] = "admin-replay"
	headers["X-Replay-Event-ID"] = record.EventID
	headers["X-Replay-Timestamp"] = time.Now().UTC().Format(time.RFC3339)
	if in.RequestID != "" {
		headers["X-Replay-Request-ID"] = in.RequestID
	}
	if in.Actor != "" {
		headers["X-Replay-Actor"] = in.Actor
	}
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

func replayActorFromRequest(r *http.Request, jwtSecret string) string {
	if v := strings.TrimSpace(r.Header.Get("X-Operator")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-User")); v != "" {
		return v
	}

	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		token := strings.TrimSpace(authz[len("Bearer "):])
		if token != "" && jwtSecret != "" {
			claims := jwt.MapClaims{}
			parsedToken, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(jwtSecret), nil
			}, jwt.WithExpirationRequired())
			if err == nil && parsedToken.Valid {
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
