package httpx

import (
	"context"
	"encoding/json"
	"net/http"

	"teampulsebridge/services/ingestion-gateway/internal/apperr"
)

func WriteError(w http.ResponseWriter, ctx context.Context, status int, err *apperr.Error, extras map[string]any) {
	if err == nil {
		err = apperr.New("httpx.WriteError", apperr.CodeInternalServerError, "internal server error", nil)
	}
	payload := map[string]any{
		"error": map[string]string{
			"code":    string(err.Code),
			"message": err.Message,
		},
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		payload["request_id"] = requestID
	}
	for k, v := range extras {
		if k == "error" || k == "request_id" {
			continue
		}
		payload[k] = v
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if encodeErr := json.NewEncoder(w).Encode(payload); encodeErr != nil {
		_ = encodeErr // Headers already sent, cannot recover.
	}
}
