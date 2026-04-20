package queue

import (
	"context"
	"fmt"
	"log/slog"

	"teampulsebridge/services/ingestion-gateway/internal/config"
)

type RuntimePublisher struct {
	Publisher     Publisher
	closers       []func() error
	statsProvider SnapshotProvider
}

func (r *RuntimePublisher) Close() error {
	for i := len(r.closers) - 1; i >= 0; i-- {
		if err := r.closers[i](); err != nil {
			return err
		}
	}
	return nil
}

func (r *RuntimePublisher) Snapshot() PublisherSnapshot {
	if r.statsProvider == nil {
		return PublisherSnapshot{}
	}
	return r.statsProvider.Snapshot()
}

func BuildRuntimePublisher(ctx context.Context, cfg config.Config, logger *slog.Logger, options AsyncPublisherOptions) (*RuntimePublisher, error) {
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

	options.Backpressure.Enabled = cfg.QueueBackpressureEnabled
	options.Backpressure.SoftLimitRatio = float64(cfg.QueueBackpressureSoftLimitPercent) / 100
	options.Backpressure.HardLimitRatio = float64(cfg.QueueBackpressureHardLimitPercent) / 100
	options.Backpressure.FailureRatioThreshold = float64(cfg.QueueFailureBudgetPercent) / 100
	options.Backpressure.FailureWindow = cfg.QueueFailureBudgetWindow
	options.Backpressure.MinSamples = cfg.QueueFailureBudgetMinSamples

	async := NewAsyncPublisherWithOptions(base, cfg.QueueBuffer, logger, options)
	r.closers = append(r.closers, async.Close)
	r.Publisher = async
	r.statsProvider = async
	return r, nil
}
