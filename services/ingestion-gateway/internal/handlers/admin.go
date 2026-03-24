package handlers

import (
	"net/http"

	"teampulsebridge/services/ingestion-gateway/internal/config"
)

type AdminHandler struct {
	cfg config.Config
}

func NewAdminHandler(cfg config.Config) *AdminHandler {
	return &AdminHandler{cfg: cfg}
}

func (h *AdminHandler) Configz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":             "ingestion-gateway",
		"queue_backend":       h.cfg.QueueBackend,
		"admin_auth_enabled":  h.cfg.AdminAuthEnabled,
		"request_timeout_sec": h.cfg.RequestTimeoutSec,
		"queue_buffer":        h.cfg.QueueBuffer,
	})
}
