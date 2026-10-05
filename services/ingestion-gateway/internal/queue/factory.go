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
	var errs []error
	for i := len(r.closers) - 1; i >= 0; i-- {
		if err := r.closers[i](); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("multiple publisher close errors: %v", errs)
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
		batchDelay := time.Duration(cfg.PubSubBatchDelayMs) * time.Millisecond
		if batchDelay <= 0 {
			batchDelay = 20 * time.Millisecond
		}
		batchCount := cfg.PubSubBatchCountThreshold
		if batchCount <= 0 {
			batchCount = 500
		}
		batchBytes := cfg.PubSubBatchByteThreshold
		if batchBytes <= 0 {
			batchBytes = 1 << 20
		}
		goroutines := cfg.PubSubPublishGoroutines
		if goroutines <= 0 {
			goroutines = 16
		}
		pub, err := NewPubSubPublisher(ctx, cfg.PubSubProjectID, cfg.PubSubTopicID, logger,
			WithPublishTimeout(time.Duration(cfg.PubSubPublishTimeoutSec)*time.Second),
			WithPublishGoroutines(goroutines),
			WithPublishFlowControl(cfg.PubSubMaxOutstandingMessages, cfg.PubSubMaxOutstandingBytes, cfg.PubSubFlowControlBehavior),
			WithPublishBatching(batchDelay, batchCount, batchBytes),
		)
		if err != nil {
			return nil, fmt.Errorf("init pubsub publisher: %w", err)
		}
		threshold := cfg.CircuitBreakerThreshold
		if threshold <= 0 {
			threshold = 25
		}
		recovery := time.Duration(cfg.CircuitBreakerRecoverySec) * time.Second
		if recovery <= 0 {
			recovery = 8 * time.Second
		}
		cb := resilience.NewCircuitBreaker(threshold, recovery)
		base = NewCircuitBreakerPublisher(pub, cb, logger)
		r.closers = append(r.closers, func() error { _ = cb.Close(); return pub.Close() })
		logger.Info("pubsub publisher configured",
			"publish_goroutines", goroutines,
			"batch_delay_ms", int(batchDelay/time.Millisecond),
			"batch_count", batchCount,
			"cb_threshold", threshold,
			"cb_recovery_sec", int(recovery/time.Second),
		)
	default:
		base = NewLogPublisher(logger)
	}

	// PII scrubbing runs INSIDE queue workers (wraps base, async wraps that),
	// so the HTTP hot path only enqueues. When PII_SCRUB_ASYNC=false we keep
	// the legacy outer wrap for strict sync semantics.
	scrubAsync := cfg.PIIScrubAsync
	if cfg.PIIScrubbingEnabled && scrubAsync {
		scrubber := NewStructuralScrubber()
		maxBytes := cfg.PIIMaxScrubBytes
		if maxBytes <= 0 {
			maxBytes = 262144
		}
		base = NewSizeGatedScrubPublisher(base, maxBytes, ScrubEmails, ScrubTokens, scrubber.Scrub)
		logger.Info("structural PII scrubbing enabled in queue workers (async)")
	}

	options.WorkerCount = cfg.QueueWorkers
	if options.WorkerCount <= 0 {
		options.WorkerCount = 16
	}
	// Safety: bulkhead fans out workers per source, so cap shared default.
	if cfg.QueueBulkheadEnabled && options.WorkerCount > 32 {
		options.WorkerCount = 32
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
		maxSources := cfg.QueueBulkheadMaxSources
		if maxSources <= 0 {
			maxSources = 8
		}
		// Memory bound guard: bufferPerSource * maxSources shouldn't explode.
		if mem := bufferPerSource * maxSources; mem > cfg.QueueBuffer*4 && cfg.QueueBuffer > 0 {
			bufferPerSource = cfg.QueueBuffer * 4 / maxSources
			if bufferPerSource < 64 {
				bufferPerSource = 64
			}
			logger.Warn("bulkhead buffer clamped to memory bound",
				"buffer_per_source", bufferPerSource, "max_sources", maxSources)
		}
		bulkhead := NewBulkheadPublisherWithMaxSources(base, bufferPerSource, logger, options, maxSources)
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

	if cfg.PIIScrubbingEnabled && !scrubAsync {
		scrubber := NewStructuralScrubber()
		r.Publisher = NewTransformingPublisher(r.Publisher, ScrubEmails, ScrubTokens, scrubber.Scrub)
		logger.Info("structural PII scrubbing enabled on request path (sync legacy)")
	}

	return r, nil
}
