package queue

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sync"
)

type SourcePublisher struct {
	queue  *AsyncPublisher
	source string
}

type BulkheadPublisher struct {
	mu              sync.RWMutex
	sources         map[string]*SourcePublisher
	logger          *slog.Logger
	bufferPerSource int
	inner           Publisher
	options         AsyncPublisherOptions
	softLimit       int
	hardLimit       int
}

func NewBulkheadPublisher(inner Publisher, bufferPerSource int, logger *slog.Logger, options AsyncPublisherOptions) *BulkheadPublisher {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if bufferPerSource <= 0 {
		bufferPerSource = 1
	}
	options = options.withDefaults()

	return &BulkheadPublisher{
		sources:         make(map[string]*SourcePublisher),
		logger:          logger,
		bufferPerSource: bufferPerSource,
		inner:           inner,
		options:         options,
		softLimit:       limitFromRatio(bufferPerSource, options.Backpressure.SoftLimitRatio),
		hardLimit:       limitFromRatio(bufferPerSource, options.Backpressure.HardLimitRatio),
	}
}

func (b *BulkheadPublisher) GetOrCreateSource(source string) *SourcePublisher {
	b.mu.RLock()
	sp, ok := b.sources[source]
	b.mu.RUnlock()
	if ok {
		return sp
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if sp, ok = b.sources[source]; ok {
		return sp
	}

	queue := NewAsyncPublisherWithOptions(b.inner, b.bufferPerSource, b.logger, b.options)
	sp = &SourcePublisher{
		queue:  queue,
		source: source,
	}
	b.sources[source] = sp

	b.logger.Info("bulkhead source queue created", "source", source, "buffer", b.bufferPerSource)
	return sp
}

func (b *BulkheadPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	sp := b.GetOrCreateSource(source)
	snapshot := sp.queue.Snapshot()

	if b.options.Backpressure.Enabled && snapshot.Depth >= b.hardLimit {
		b.emitBackpressure(ctx, source, "hard_limit", snapshot)
		return ErrQueueFull
	}

	if b.options.Backpressure.Enabled && snapshot.Depth >= b.softLimit {
		b.emitBackpressure(ctx, source, "soft_limit", snapshot)
	}

	err := sp.queue.Publish(ctx, source, body, headers)
	if err != nil {
		if b.options.Hooks.OnPublish != nil {
			b.options.Hooks.OnPublish(ctx, source, "failed", sp.queue.Snapshot())
		}
		return err
	}

	if b.options.Hooks.OnPublish != nil {
		b.options.Hooks.OnPublish(ctx, source, "queued", sp.queue.Snapshot())
	}
	return nil
}

func (b *BulkheadPublisher) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var errs []error
	for source, sp := range b.sources {
		if err := sp.queue.Close(); err != nil {
			errs = append(errs, fmt.Errorf("source %s: %w", source, err))
			b.logger.Error("bulkhead source queue close failed", "source", source, "error", err)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("multiple close errors: %v", errs)
}

func (b *BulkheadPublisher) Snapshot() PublisherSnapshot {
	snapshots := b.SourceSnapshots()
	aggregate := PublisherSnapshot{}
	var weightedFailures float64
	for _, snapshot := range snapshots {
		aggregate.Depth += snapshot.Depth
		aggregate.Capacity += snapshot.Capacity
		aggregate.RecentSamples += snapshot.RecentSamples
		weightedFailures += snapshot.FailureRatio * float64(snapshot.RecentSamples)
	}
	if aggregate.Capacity > 0 {
		aggregate.UsageRatio = clampRatio(float64(aggregate.Depth) / float64(aggregate.Capacity))
	}
	if aggregate.RecentSamples > 0 {
		aggregate.FailureRatio = clampRatio(weightedFailures / float64(aggregate.RecentSamples))
	}
	return aggregate
}

func (b *BulkheadPublisher) SourceSnapshots() map[string]PublisherSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()

	snapshots := make(map[string]PublisherSnapshot, len(b.sources))
	for source, sp := range b.sources {
		snapshots[source] = sp.queue.Snapshot()
	}
	return snapshots
}

func (b *BulkheadPublisher) HealthCheck(ctx context.Context) error {
	if err := b.inner.HealthCheck(ctx); err != nil {
		return err
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	for source, sp := range b.sources {
		if err := sp.queue.HealthCheck(ctx); err != nil {
			return fmt.Errorf("source queue %s unhealthy: %w", source, err)
		}
	}
	return nil
}

func (b *BulkheadPublisher) emitBackpressure(ctx context.Context, source, action string, snapshot PublisherSnapshot) {
	if b.options.Hooks.OnBackpressure != nil {
		b.options.Hooks.OnBackpressure(ctx, source, action, snapshot)
	}
}

func limitFromRatio(capacity int, ratio float64) int {
	if capacity <= 0 {
		return 1
	}
	if ratio <= 0 {
		return 1
	}
	limit := int(math.Ceil(float64(capacity) * ratio))
	if limit < 1 {
		return 1
	}
	if limit > capacity {
		return capacity
	}
	return limit
}
