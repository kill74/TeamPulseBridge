package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/platform/resilience"
)

type RuntimePublisher struct {
	Publisher           Publisher
	closers             []func() error
	statsProvider       SnapshotProvider
	sourceStatsProvider SourceSnapshotProvider
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

func (r *RuntimePublisher) SourceSnapshots() map[string]PublisherSnapshot {
	if r.sourceStatsProvider == nil {
		return nil
	}
	return r.sourceStatsProvider.SourceSnapshots()
}

func BuildRuntimePublisher(ctx context.Context, cfg config.Config, logger *slog.Logger, options AsyncPublisherOptions) (*RuntimePublisher, error) {
	var base Publisher
	r := &RuntimePublisher{}

	switch cfg.QueueBackend {
	case "pubsub":
		pub, err := NewPubSubPublisher(ctx, cfg.PubSubProjectID, cfg.PubSubTopicID, logger,
			WithPublishTimeout(time.Duration(cfg.PubSubPublishTimeoutSec)*time.Second),
			WithPublishGoroutines(cfg.PubSubPublishGoroutines),
			WithPublishFlowControl(cfg.PubSubMaxOutstandingMessages, cfg.PubSubMaxOutstandingBytes, cfg.PubSubFlowControlBehavior),
		)
		if err != nil {
			return nil, fmt.Errorf("init pubsub publisher: %w", err)
		}
		cb := resilience.NewCircuitBreaker(5, 30*time.Second)
		base = NewCircuitBreakerPublisher(pub, cb, logger)
		r.closers = append(r.closers, pub.Close)
	default:
		base = NewLogPublisher(logger)
	}

	options.WorkerCount = cfg.QueueWorkers
	if options.WorkerCount <= 0 {
		options.WorkerCount = 1
	}
	options.Backpressure.Enabled = cfg.QueueBackpressureEnabled
	options.Backpressure.SoftLimitRatio = float64(cfg.QueueBackpressureSoftLimitPercent) / 100
	options.Backpressure.HardLimitRatio = float64(cfg.QueueBackpressureHardLimitPercent) / 100
	options.Backpressure.FailureRatioThreshold = float64(cfg.QueueFailureBudgetPercent) / 100
	options.Backpressure.FailureWindow = cfg.QueueFailureBudgetWindow
	options.Backpressure.MinSamples = cfg.QueueFailureBudgetMinSamples

	if cfg.QueueBulkheadEnabled {
		bufferPerSource := cfg.QueueBulkheadBufferPerSource
		if bufferPerSource <= 0 {
			bufferPerSource = cfg.QueueBuffer
		}
		bulkhead := NewBulkheadPublisher(base, bufferPerSource, logger, options)
		r.closers = append(r.closers, bulkhead.Close)
		r.Publisher = bulkhead
		r.statsProvider = bulkhead
		r.sourceStatsProvider = bulkhead
	} else {
		async := NewAsyncPublisherWithOptions(base, cfg.QueueBuffer, logger, options)
		r.closers = append(r.closers, async.Close)
		r.Publisher = async
		r.statsProvider = async
	}

	if cfg.PIIScrubbingEnabled {
		scrubber := NewStructuralScrubber()
		r.Publisher = NewTransformingPublisher(r.Publisher, ScrubEmails, ScrubTokens, scrubber.Scrub)
		logger.Info("structural PII scrubbing enabled for outbound queue publishes")
	}

	return r, nil
}
