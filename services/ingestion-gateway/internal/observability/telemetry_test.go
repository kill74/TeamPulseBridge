package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger(t *testing.T) {
	os.Setenv("LOG_LEVEL", "debug")
	defer os.Unsetenv("LOG_LEVEL")

	logger := NewLogger("test-service", "test-env", "v1.0.0")
	assert.NotNil(t, logger)
	
	// We can't easily capture stdout here without more complex setup,
	// but we can verify it doesn't panic and is configured.
	logger.Info("test message")
}

func TestSetup(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	
	// Clear OTEL env vars to ensure predictable behavior
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	
	tel, err := Setup(ctx, logger, "test-service")
	require.NoError(t, err)
	defer tel.Shutdown(ctx)
	
	assert.NotNil(t, tel.MetricsHandler)
	assert.NotNil(t, tel.WebhookCounter)
	assert.NotNil(t, tel.HTTPDurationHistogram)
}

func TestLogLevelFromEnv(t *testing.T) {
	tests := []struct {
		env   string
		level slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"invalid", slog.LevelInfo},
	}

	for _, tt := range tests {
		os.Setenv("LOG_LEVEL", tt.env)
		assert.Equal(t, tt.level, logLevelFromEnv())
	}
	os.Unsetenv("LOG_LEVEL")
}
