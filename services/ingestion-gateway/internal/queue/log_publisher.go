package queue

import (
	"context"
	"log/slog"
)

// LogPublisher is a local/dev publisher implementation.
type LogPublisher struct {
	logger *slog.Logger
}

func NewLogPublisher(logger *slog.Logger) *LogPublisher {
	return &LogPublisher{logger: logger}
}

func (p *LogPublisher) Publish(_ context.Context, source string, body []byte, _ map[string]string) error {
	p.logger.Info("queued event", "source", source, "bytes", len(body))
	return nil
}

func (p *LogPublisher) Close() error {
	return nil
}
