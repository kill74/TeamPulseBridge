package observability

import (
	"log/slog"
	"os"
	"strings"
)

func NewLogger(serviceName, environment, version string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevelFromEnv(),
		AddSource: true,
	})
	return slog.New(h).With(
		"service", serviceName,
		"environment", environment,
		"version", version,
	)
}

func logLevelFromEnv() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
