package observability

import (
	"log/slog"
	"os"
	"strings"
)

type Logger struct {
	*slog.Logger
	level *slog.LevelVar
}

func NewLogger(serviceName, environment, version string) *Logger {
	lVar := &slog.LevelVar{}
	lVar.Set(logLevelFromEnv())

	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     lVar,
		AddSource: true,
	})
	l := slog.New(h).With(
		"service", serviceName,
		"environment", environment,
		"version", version,
	)
	return &Logger{
		Logger: l,
		level:  lVar,
	}
}

func (l *Logger) SetLevel(level slog.Level) {
	l.level.Set(level)
}

func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
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

func logLevelFromEnv() slog.Level {
	return ParseLevel(os.Getenv("LOG_LEVEL"))
}
