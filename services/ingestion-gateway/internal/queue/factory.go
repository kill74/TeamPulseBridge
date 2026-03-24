package queue

import (
	"context"
	"fmt"
	"log/slog"

	"teampulsebridge/services/ingestion-gateway/internal/config"
)

type RuntimePublisher struct {
	Publisher Publisher
	closers   []func() error
}

func (r *RuntimePublisher) Close() error {
	for i := len(r.closers) - 1; i >= 0; i-- {
		if err := r.closers[i](); err != nil {
			return err
		}
	}
	return nil
}

func BuildRuntimePublisher(ctx context.Context, cfg config.Config, logger *slog.Logger) (*RuntimePublisher, error) {
	var base Publisher
	r := &RuntimePublisher{}

	switch cfg.QueueBackend {
	case "pubsub":
		pub, err := NewPubSubPublisher(ctx, cfg.PubSubProjectID, cfg.PubSubTopicID, logger)
		if err != nil {
			return nil, fmt.Errorf("init pubsub publisher: %w", err)
		}
		base = pub
		r.closers = append(r.closers, pub.Close)
	default:
		base = NewLogPublisher(logger)
	}

	async := NewAsyncPublisher(base, cfg.QueueBuffer, logger)
	r.closers = append(r.closers, func() error {
		async.Close()
		return nil
	})
	r.Publisher = async
	return r, nil
}
