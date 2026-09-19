package handlers

import (
	"bytes"
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
	"sync"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/dedup"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/httpx"
	"teampulsebridge/services/ingestion-gateway/internal/platform/signature"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
	"teampulsebridge/services/ingestion-gateway/internal/schema"
	"teampulsebridge/services/ingestion-gateway/internal/securityaudit"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

// bodyBufferPool reuses up-to-1MiB buffers across webhook requests to cut GC churn.
var bodyBufferPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 32*1024)
		return &b
	},
}

// acceptedResponse avoids per-request map allocations on the hot 202 path.
type acceptedResponse struct {
	Status  string `json:"status"`
	EventID string `json:"event_id,omitempty"`
}

type WebhookHandler struct {
	cfg             config.Config
	publisher       queue.Publisher
	logger          *slog.Logger
	record          func(ctx context.Context, source string, status int)
	deduper         dedup.Store
	failed          failstore.Store
	onSecurityEvent func(r *http.Request, event SecurityEvent)
	validator       *schema.Validator
}

type SecurityEvent struct {
	Category string
	Source   string
	Reason   string
	Status   int
	Actor    string
}

type slackChallenge struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
}

func NewWebhookHandler(cfg config.Config, publisher queue.Publisher, logger *slog.Logger, record func(ctx context.Context, source string, status int)) *WebhookHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return NewWebhookHandlerWithDependencies(cfg, publisher, logger, record, nil, nil, nil, nil)
}

func NewWebhookHandlerWithDependencies(
	cfg config.Config,
	publisher queue.Publisher,
	logger *slog.Logger,
	record func(ctx context.Context, source string, status int),
	deduper dedup.Store,
	failed failstore.Store,
	onSecurityEvent func(r *http.Request, event SecurityEvent),
	validator *schema.Validator,
) *WebhookHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &WebhookHandler{
		cfg:             cfg,
		publisher:       publisher,
		logger:          logger,
		record:          record,
		deduper:         deduper,
		failed:          failed,
		onSecurityEvent: onSecurityEvent,
		validator:       validator,
	}
}

func (h *WebhookHandler) HandleSlack(w http.ResponseWriter, r *http.Request) {
	if h.rejectMethod(w, r, "slack", http.MethodPost) {
		return
	}
	if err := r.Context().Err(); err != nil {
		h.observe(r.Context(), "slack", 499)
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
	// Single decode yields both challenge and event_id to avoid 2-3 JSON passes.
	var mini struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		EventID   string `json:"event_id"`
	}
	if err := json.Unmarshal(body, &mini); err == nil && mini.Type == "url_verification" && mini.Challenge != "" {
		h.observe(r.Context(), "slack", http.StatusOK)
		writeJSON(w, http.StatusOK, map[string]string{"challenge": mini.Challenge})
		return
	}

	h.publishAndAck(w, r, "slack", body)
}

func (h *WebhookHandler) HandleGitHub(w http.ResponseWriter, r *http.Request) {
	if h.rejectMethod(w, r, "github", http.MethodPost) {
		return
	}
	if err := r.Context().Err(); err != nil {
		h.observe(r.Context(), "github", 499)
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
		h.observe(r.Context(), "gitlab", 499)
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
		h.observe(r.Context(), "teams", 499)
		return
	}
	if token := r.URL.Query().Get("validationToken"); token != "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, writeErr := w.Write([]byte(token))
		h.observe(r.Context(), "teams", http.StatusOK)
		if writeErr != nil {
			return
		}
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
	if h.validator != nil {
		if err := h.validator.Validate(source, body); err != nil {
			h.observe(r.Context(), source, http.StatusBadRequest)
			appErr := apperr.New("handlers.publishAndAck", apperr.CodeInvalidRequestBody, "schema validation failed", err)
			h.respondError(w, r.Context(), http.StatusBadRequest, appErr, "")
			h.logger.Warn("schema validation failed",
				"request_id", httpx.RequestIDFromContext(r.Context()),
				"source", source,
				"error", err,
			)
			return
		}
	}

	eventID := deriveEventID(source, r, body)
	if eventID != "" && h.deduper != nil && dedupSeen(r.Context(), h.deduper, source+":"+eventID) {
		h.observe(r.Context(), source, http.StatusAccepted)
		h.logger.Info("duplicate webhook event ignored",
			"request_id", httpx.RequestIDFromContext(r.Context()),
			"source", source,
			"event_id", eventID,
			"error_code", apperr.CodeDuplicateEvent,
		)
		writeJSON(w, http.StatusAccepted, acceptedResponse{
			Status:  "accepted",
			EventID: eventID,
		})
		return
	}

	headers := make(map[string]string, 5)
	if ct := r.Header.Get("Content-Type"); ct != "" {
		headers["Content-Type"] = ct
	}
	if ua := r.Header.Get("User-Agent"); ua != "" {
		headers["User-Agent"] = ua
	}
	if eventID != "" {
		headers["X-Event-ID"] = eventID
	}
	if requestID := httpx.RequestIDFromContext(r.Context()); requestID != "" {
		headers["X-Request-Id"] = requestID
	}
	if traceparent := strings.TrimSpace(r.Header.Get("Traceparent")); traceparent != "" {
		headers["Traceparent"] = traceparent
	}
	if err := h.publisher.Publish(r.Context(), source, body, headers); err != nil {
		// Only release the dedup reservation on retriable enqueue failures so
		// duplicates cannot slip through when the failure was terminal.
		if errors.Is(err, queue.ErrQueueFull) || errors.Is(err, queue.ErrQueueThrottled) {
			h.forgetDedupKey(source, eventID)
		}
		status := http.StatusInternalServerError
		errCode := apperr.CodePublishFailed
		retryAfter := 0
		if errors.Is(err, queue.ErrQueueThrottled) {
			status = http.StatusTooManyRequests
			errCode = apperr.CodeQueueThrottled
			retryAfter = h.cfg.QueueThrottleRetryAfterSec
			if retryAfter < 1 {
				retryAfter = 2
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		} else if errors.Is(err, queue.ErrQueueFull) {
			status = http.StatusServiceUnavailable
			errCode = apperr.CodeQueueFull
			// Fail fast with an explicit backoff hint so clients don't hammer us.
			retryAfter = h.cfg.QueueThrottleRetryAfterSec
			if retryAfter < 1 {
				retryAfter = 2
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
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
	writeJSON(w, http.StatusAccepted, acceptedResponse{Status: "accepted", EventID: eventID})
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
	h.recordSecurityEvent(r, SecurityEvent{
		Category: "webhook_request_rejected",
		Source:   source,
		Reason:   "webhook_method_not_allowed",
		Status:   http.StatusMethodNotAllowed,
	})
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
	h.recordSecurityEvent(r, SecurityEvent{
		Category: "webhook_request_rejected",
		Source:   source,
		Reason:   "webhook_auth_failed",
		Status:   http.StatusUnauthorized,
	})
	h.logger.Warn("webhook authentication failed",
		"request_id", httpx.RequestIDFromContext(r.Context()),
		"source", source,
		"error", cause,
		"error_code", appErr.Code,
	)
	h.respondError(w, r.Context(), http.StatusUnauthorized, appErr, "")
	return true
}

func readBody(w http.ResponseWriter, r *http.Request) (body []byte, appErr *apperr.Error, status int) {
	// Use a pooled buffer + CopyN so bursts of 1MiB bodies don't churn the GC,
	// and so oversized bodies are detected without growing to 2x.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes+1)
	defer func() { _ = r.Body.Close() }()
	bufPtr := bodyBufferPool.Get().(*[]byte)
	buf := (*bufPtr)[:0]
	defer func() {
		*bufPtr = buf[:0]
		// Don't return oversized buffers to the pool.
		if cap(*bufPtr) <= 2*maxRequestBodyBytes {
			bodyBufferPool.Put(bufPtr)
		}
	}()
	// Reuse via bytes.Buffer semantics without extra allocs.
	tmp := bytes.NewBuffer(buf)
	n, err := io.CopyN(tmp, r.Body, maxRequestBodyBytes+1)
	_ = n
	if err != nil && !errors.Is(err, io.EOF) {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || tmp.Len() > maxRequestBodyBytes {
			return nil, apperr.New("handlers.readBody", apperr.CodePayloadTooLarge, "request body too large", err), http.StatusRequestEntityTooLarge
		}
		return nil, apperr.New("handlers.readBody", apperr.CodeInvalidRequestBody, "invalid request body", err), http.StatusBadRequest
	}
	if tmp.Len() > maxRequestBodyBytes {
		return nil, apperr.New("handlers.readBody", apperr.CodePayloadTooLarge, "request body too large", errors.New("body exceeds 1MiB")), http.StatusRequestEntityTooLarge
	}
	// Copy out: pooled buffer is reused, returned body is owned by caller.
	out := make([]byte, tmp.Len())
	copy(out, tmp.Bytes())
	return out, nil, http.StatusOK
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Debug("json encode failed in writeJSON", "error", err)
	}
}

func (h *WebhookHandler) respondError(w http.ResponseWriter, ctx context.Context, status int, err *apperr.Error, eventID string) {
	var extras map[string]any
	if eventID != "" {
		extras = map[string]any{"event_id": eventID}
	}
	httpx.WriteError(w, ctx, status, err, extras)
}

func (h *WebhookHandler) forgetDedupKey(source, eventID string) {
	if h.deduper == nil || eventID == "" {
		return
	}
	// Best-effort, non-blocking: redis path has a 100ms budget internally.
	h.deduper.Forget(source + ":" + eventID)
}

// dedupSeen prefers the context-aware fast path (100ms budget, request
// cancellation) when the store supports it.
func dedupSeen(ctx context.Context, store dedup.Store, key string) bool {
	if cs, ok := store.(dedup.ContextStore); ok {
		return cs.SeenWithContext(ctx, key)
	}
	return store.Seen(key)
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

func (h *WebhookHandler) recordSecurityEvent(r *http.Request, event SecurityEvent) {
	if h.onSecurityEvent == nil {
		return
	}
	h.onSecurityEvent(r, event)
}

func SecurityAuditRecord(r *http.Request, event SecurityEvent) securityaudit.SaveInput {
	actor := strings.TrimSpace(event.Actor)
	if actor == "" {
		actor = securityActorFromRequest(r)
	}
	return securityaudit.SaveInput{
		Category:   strings.TrimSpace(event.Category),
		Outcome:    "rejected",
		Source:     strings.TrimSpace(event.Source),
		Reason:     strings.TrimSpace(event.Reason),
		Path:       r.URL.Path,
		HTTPStatus: event.Status,
		RequestID:  httpx.RequestIDFromContext(r.Context()),
		Actor:      actor,
	}
}

func securityActorFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if v := strings.TrimSpace(r.Header.Get("X-Operator")); v != "" {
		return v
	}
	if v := strings.TrimSpace(r.Header.Get("X-User")); v != "" {
		return v
	}
	return ""
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
	// Fast-path: avoid a full map[string]RawMessage alloc when the field is absent.
	if !bytes.Contains(body, []byte(strconv.Quote(field))) && !bytes.Contains(body, []byte(field)) {
		return ""
	}
	// Single streaming pass: decode top-level keys, skip values without copying all of them.
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return ""
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return ""
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return ""
		}
		key, ok := keyTok.(string)
		if !ok {
			return ""
		}
		if key == field {
			var value string
			if err := dec.Decode(&value); err != nil {
				return ""
			}
			return strings.TrimSpace(value)
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return ""
		}
	}
	return ""
}

func fallbackEventID(source string, body []byte) string {
	// For tiny bodies hash everything; for large bodies hash prefix+len so we
	// don't SHA256 a full MiB on every header-less slack/teams request.
	if len(body) <= 32*1024 {
		sum := sha256.Sum256(body)
		return source + "_" + hex.EncodeToString(sum[:])
	}
	h := sha256.New()
	_, _ = h.Write(body[:8192])
	_, _ = h.Write([]byte(strconv.Itoa(len(body))))
	sum := h.Sum(nil)
	return source + "_" + hex.EncodeToString(sum)
}
