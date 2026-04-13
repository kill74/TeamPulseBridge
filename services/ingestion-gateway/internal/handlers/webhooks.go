package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/config"
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
}

type slackChallenge struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
}

func NewWebhookHandler(cfg config.Config, publisher queue.Publisher, logger *slog.Logger, record func(ctx context.Context, source string, status int)) *WebhookHandler {
	return &WebhookHandler{cfg: cfg, publisher: publisher, logger: logger, record: record}
}

func (h *WebhookHandler) HandleSlack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}

	if err := signature.ValidateSlack(
		h.cfg.SlackSigningSecret,
		r.Header.Get("X-Slack-Request-Timestamp"),
		string(body),
		r.Header.Get("X-Slack-Signature"),
		time.Now().UTC(),
		5*time.Minute,
	); err != nil {
		h.observe(r.Context(), "slack", http.StatusUnauthorized)
		http.Error(w, err.Error(), http.StatusUnauthorized)
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	if err := signature.ValidateGitHub(h.cfg.GitHubWebhookSecret, body, r.Header.Get("X-Hub-Signature-256")); err != nil {
		h.observe(r.Context(), "github", http.StatusUnauthorized)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	h.publishAndAck(w, r, "github", body)
}

func (h *WebhookHandler) HandleGitLab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.Context().Err(); err != nil {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	if err := signature.ValidateGitLab(h.cfg.GitLabWebhookToken, r.Header.Get("X-Gitlab-Token")); err != nil {
		h.observe(r.Context(), "gitlab", http.StatusUnauthorized)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	h.publishAndAck(w, r, "gitlab", body)
}

func (h *WebhookHandler) HandleTeams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	body, ok := readBody(w, r)
	if !ok {
		return
	}
	if err := signature.ValidateTeamsClientState(h.cfg.TeamsClientState, r.Header.Get("X-Client-State")); err != nil {
		h.observe(r.Context(), "teams", http.StatusUnauthorized)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	h.publishAndAck(w, r, "teams", body)
}

func (h *WebhookHandler) publishAndAck(w http.ResponseWriter, r *http.Request, source string, body []byte) {
	headers := map[string]string{
		"Content-Type": r.Header.Get("Content-Type"),
		"User-Agent":   r.Header.Get("User-Agent"),
	}
	if err := h.publisher.Publish(r.Context(), source, body, headers); err != nil {
		status := http.StatusInternalServerError
		if err == queue.ErrQueueFull {
			status = http.StatusServiceUnavailable
		}
		h.observe(r.Context(), source, status)
		h.logger.Error("publish failed",
			"request_id", httpx.RequestIDFromContext(r.Context()),
			"source", source,
			"status", status,
			"error", err,
		)
		http.Error(w, "failed to enqueue event", status)
		return
	}
	h.observe(r.Context(), source, http.StatusAccepted)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *WebhookHandler) observe(ctx context.Context, source string, status int) {
	if h.record != nil {
		h.record(ctx, source, status)
	}
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		_ = err // Headers already sent, cannot recover
	}
}
