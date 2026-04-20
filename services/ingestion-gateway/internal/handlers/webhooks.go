package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/dedup"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/httpx"
	"teampulsebridge/services/ingestion-gateway/internal/platform/signature"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

const maxBodyBytes = 1 << 20 // 1 MiB

type WebhookHandler struct {
	cfg       config.Config
	publisher queue.Publisher
	logger    *slog.Logger
	record    func(ctx context.Context, source string, status int)
	deduper   dedupStore
	failed    failstore.Store
}

type slackChallenge struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
}

func NewWebhookHandler(cfg config.Config, publisher queue.Publisher, logger *slog.Logger, record func(ctx context.Context, source string, status int)) *WebhookHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	dedupTTL := time.Duration(cfg.DedupTTLSeconds) * time.Second
	if dedupTTL <= 0 {
		dedupTTL = 5 * time.Minute
	}
	deduper := dedup.NewMemory(cfg.DedupEnabled, dedupTTL)

	var failedStore failstore.Store
	if cfg.FailedStoreEnabled {
		store, err := failstore.NewFileStore(cfg.FailedStorePath)
		if err != nil {
			logger.Error("failed event store disabled due to invalid configuration",
				"path", cfg.FailedStorePath,
				"error", err,
				"error_code", apperr.CodeFailedEventStore,
			)
		} else {
			failedStore = store
		}
	}

	return NewWebhookHandlerWithDependencies(cfg, publisher, logger, record, deduper, failedStore)
}

func NewWebhookHandlerWithDependencies(
	cfg config.Config,
	publisher queue.Publisher,
	logger *slog.Logger,
	record func(ctx context.Context, source string, status int),
	deduper dedupStore,
	failed failstore.Store,
) *WebhookHandler {
	return &WebhookHandler{
		cfg:       cfg,
		publisher: publisher,
		logger:    logger,
		record:    record,
		deduper:   deduper,
		failed:    failed,
	}
}

func (h *WebhookHandler) HandleSlack(w http.ResponseWriter, r *http.Request) {
	if h.rejectMethod(w, r, "slack", http.MethodPost) {
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	body, readErr, status := readBody(w, r)
	if readErr != nil {
		h.observe(r.Context(), "slack", status)
		h.respondError(w, r.Context(), status, readErr, "")
		return
	}

	if h.rejectUnauthorized(w, r, "slack", signature.ValidateSlack(
		h.cfg.SlackSigningSecret,
		r.Header.Get("X-Slack-Request-Timestamp"),
		string(body),
		r.Header.Get("X-Slack-Signature"),
		time.Now().UTC(),
		5*time.Minute,
	)) {
		return
	}

	// Slack URL verification during webhook registration.
	var c slackChallenge
	if err := json.Unmarshal(body, &c); err == nil && c.Type == "url_verification" && c.Challenge != "" {
		h.observe(r.Context(), "slack", http.StatusOK)
		writeJSON(w, http.StatusOK, map[string]string{"challenge": c.Challenge})
		return
	}

	h.publishAndAck(w, r, "slack", body)
}

func (h *WebhookHandler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
	if h.rejectMethod(w, r, "github", http.MethodPost) {
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	body, readErr, status := readBody(w, r)
	if readErr != nil {
		h.observe(r.Context(), "github", status)
		h.respondError(w, r.Context(), status, readErr, "")
		return
	}
	if h.rejectUnauthorized(w, r, "github", signature.ValidateGitHub(h.cfg.GitHubWebhookSecret, body, r.Header.Get("X-Hub-Signature-256"))) {
		return
	}
	h.publishAndAck(w, r, "github", body)
}

func (h *WebhookHandler) HandleGitLab(w http.ResponseWriter, r *http.Request) {
	if h.rejectMethod(w, r, "gitlab", http.MethodPost) {
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	body, readErr, status := readBody(w, r)
	if readErr != nil {
		h.observe(r.Context(), "gitlab", status)
		h.respondError(w, r.Context(), status, readErr, "")
		return
	}
	if h.rejectUnauthorized(w, r, "gitlab", signature.ValidateGitLab(h.cfg.GitLabWebhookToken, r.Header.Get("X-Gitlab-Token"))) {
		return
	}
	h.publishAndAck(w, r, "gitlab", body)
}

func (h *WebhookHandler) HandleTeams(w http.ResponseWriter, r *http.Request) {
	if h.rejectMethod(w, r, "teams", http.MethodGet, http.MethodPost) {
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	if token := r.URL.Query().Get("validationToken"); token != "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(token)); err != nil {
			return
		}
		h.observe(r.Context(), "teams", http.StatusOK)
		return
	}

	body, readErr, status := readBody(w, r)
	if readErr != nil {
		h.observe(r.Context(), "teams", status)
		h.respondError(w, r.Context(), status, readErr, "")
		return
	}
	if h.rejectUnauthorized(w, r, "teams", signature.ValidateTeamsClientState(h.cfg.TeamsClientState, r.Header.Get("X-Client-State"))) {
		return
	}
	h.publishAndAck(w, r, "teams", body)
}

func (h *WebhookHandler) publishAndAck(w http.ResponseWriter, r *http.Request, source string, body []byte) {
	eventID := deriveEventID(source, r, body)
	if eventID != "" && h.deduper != nil && h.deduper.Seen(source+":"+eventID) {
		h.observe(r.Context(), source, http.StatusAccepted)
		h.logger.Info("duplicate webhook event ignored",
			"request_id", httpx.RequestIDFromContext(r.Context()),
			"source", source,
			"event_id", eventID,
			"error_code", apperr.CodeDuplicateEvent,
		)
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":   "accepted",
			"event_id": eventID,
		})
		return
	}

	headers := map[string]string{
		"Content-Type": r.Header.Get("Content-Type"),
		"User-Agent":   r.Header.Get("User-Agent"),
	}
	if eventID != "" {
		headers["X-Event-ID"] = eventID
	}
	if err := h.publisher.Publish(r.Context(), source, body, headers); err != nil {
		status := http.StatusInternalServerError
		errCode := apperr.CodePublishFailed
		if errors.Is(err, queue.ErrQueueThrottled) {
			status = http.StatusTooManyRequests
			errCode = apperr.CodeQueueThrottled
			retryAfter := h.cfg.QueueThrottleRetryAfterSec
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		} else if errors.Is(err, queue.ErrQueueFull) {
			status = http.StatusServiceUnavailable
			errCode = apperr.CodeQueueFull
		}
		appErr := apperr.New("handlers.publishAndAck", errCode, "failed to enqueue event", err)

		h.persistFailedEvent(r.Context(), eventID, source, string(errCode), headers, body)
		h.observe(r.Context(), source, status)
		h.logger.Error("publish failed",
			"request_id", httpx.RequestIDFromContext(r.Context()),
			"source", source,
			"event_id", eventID,
			"status", status,
			"error", appErr,
			"error_code", appErr.Code,
		)
		h.respondError(w, r.Context(), status, appErr, eventID)
		return
	}
	h.observe(r.Context(), source, http.StatusAccepted)
	resp := map[string]string{"status": "accepted"}
	if eventID != "" {
		resp["event_id"] = eventID
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (h *WebhookHandler) observe(ctx context.Context, source string, status int) {
	if h.record != nil {
		h.record(ctx, source, status)
	}
}

func (h *WebhookHandler) rejectMethod(w http.ResponseWriter, r *http.Request, source string, allowed ...string) bool {
	for _, method := range allowed {
		if r.Method == method {
			return false
		}
	}
	allow := strings.Join(allowed, ", ")
	if allow != "" {
		w.Header().Set("Allow", allow)
	}
	appErr := apperr.New("handlers.rejectMethod", apperr.CodeMethodNotAllowed, "method not allowed", fmt.Errorf("method=%s allow=%s", r.Method, allow))
	h.observe(r.Context(), source, http.StatusMethodNotAllowed)
	h.logger.Warn("webhook method rejected",
		"request_id", httpx.RequestIDFromContext(r.Context()),
		"source", source,
		"method", r.Method,
		"allow", allow,
		"error_code", appErr.Code,
	)
	h.respondError(w, r.Context(), http.StatusMethodNotAllowed, appErr, "")
	return true
}

func (h *WebhookHandler) rejectUnauthorized(w http.ResponseWriter, r *http.Request, source string, cause error) bool {
	if cause == nil {
		return false
	}
	appErr := apperr.New("handlers.rejectUnauthorized", apperr.CodeUnauthorized, "webhook authentication failed", cause)
	h.observe(r.Context(), source, http.StatusUnauthorized)
	h.logger.Warn("webhook authentication failed",
		"request_id", httpx.RequestIDFromContext(r.Context()),
		"source", source,
		"error", cause,
		"error_code", appErr.Code,
	)
	h.respondError(w, r.Context(), http.StatusUnauthorized, appErr, "")
	return true
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, *apperr.Error, int) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, apperr.New("handlers.readBody", apperr.CodePayloadTooLarge, "request body too large", err), http.StatusRequestEntityTooLarge
		}
		return nil, apperr.New("handlers.readBody", apperr.CodeInvalidRequestBody, "invalid request body", err), http.StatusBadRequest
	}
	return body, nil, http.StatusOK
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		_ = err // Headers already sent, cannot recover
	}
}

func (h *WebhookHandler) respondError(w http.ResponseWriter, ctx context.Context, status int, err *apperr.Error, eventID string) {
	var extras map[string]any
	if eventID != "" {
		extras = map[string]any{"event_id": eventID}
	}
	httpx.WriteError(w, ctx, status, err, extras)
}

func (h *WebhookHandler) persistFailedEvent(ctx context.Context, eventID, source, reason string, headers map[string]string, body []byte) {
	if h.failed == nil {
		return
	}
	record, err := h.failed.Save(ctx, failstore.SaveInput{
		EventID: eventID,
		Source:  source,
		Reason:  reason,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		h.logger.Error("failed to persist failed event",
			"source", source,
			"event_id", eventID,
			"error", err,
			"error_code", apperr.CodeFailedEventStore,
		)
		return
	}
	h.logger.Info("failed event persisted",
		"source", source,
		"event_id", record.EventID,
		"reason", record.Reason,
		"payload_hash", record.PayloadHash,
		"failed_at", record.FailedAt,
	)
}

type dedupStore interface {
	Seen(key string) bool
}

func deriveEventID(source string, r *http.Request, body []byte) string {
	switch source {
	case "github":
		if v := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery")); v != "" {
			return v
		}
	case "gitlab":
		for _, key := range []string{"X-Gitlab-Event-UUID", "X-Gitlab-Webhook-UUID", "X-Request-Id"} {
			if v := strings.TrimSpace(r.Header.Get(key)); v != "" {
				return v
			}
		}
	case "slack":
		if v := extractJSONField(body, "event_id"); v != "" {
			return v
		}
	case "teams":
		if v := extractJSONField(body, "id"); v != "" {
			return v
		}
	}
	return fallbackEventID(source, body)
}

func extractJSONField(body []byte, field string) string {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return ""
	}
	raw, ok := doc[field]
	if !ok {
		return ""
	}
	s, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func fallbackEventID(source string, body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%s_%s", source, hex.EncodeToString(sum[:8]))
}
