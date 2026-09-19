package queue

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
	maxSources      int
	inner           Publisher
	options         AsyncPublisherOptions
	softLimit       int
	hardLimit       int
	// cachedAggregate avoids O(N) Snapshot work on every metrics scrape.
	cachedAt   atomic.Int64 // unix nano
	cachedAgg  atomic.Value // PublisherSnapshot
	allowedSet map[string]struct{}
}

// allowedSources is the closed set of webhook sources. Unknown sources are
// routed to a shared "other" queue so attacker-controlled cardinality can't
// leak goroutines/channels.
var allowedSources = map[string]struct{}{
	"slack":  {},
	"github": {},
	"gitlab": {},
	"teams":  {},
}

func NewBulkheadPublisher(inner Publisher, bufferPerSource int, logger *slog.Logger, options AsyncPublisherOptions) *BulkheadPublisher {
	return NewBulkheadPublisherWithMaxSources(inner, bufferPerSource, logger, options, 8)
}

func NewBulkheadPublisherWithMaxSources(inner Publisher, bufferPerSource int, logger *slog.Logger, options AsyncPublisherOptions, maxSources int) *BulkheadPublisher {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if bufferPerSource <= 0 {
		bufferPerSource = 1
	}
	if maxSources <= 0 {
		maxSources = 8
	}
	options = options.withDefaults()

	return &BulkheadPublisher{
		sources:         make(map[string]*SourcePublisher),
		logger:          logger,
		bufferPerSource: bufferPerSource,
		maxSources:      maxSources,
		inner:           inner,
		options:         options,
		softLimit:       limitFromRatio(bufferPerSource, options.Backpressure.SoftLimitRatio),
		hardLimit:       limitFromRatio(bufferPerSource, options.Backpressure.HardLimitRatio),
		allowedSet:      allowedSources,
	}
}

func (b *BulkheadPublisher) normalizeSource(source string) string {
	s := strings.ToLower(strings.TrimSpace(source))
	if _, ok := b.allowedSet[s]; ok {
		return s
	}
	return "other"
}

func (b *BulkheadPublisher) GetOrCreateSource(source string) *SourcePublisher {
	source = b.normalizeSource(source)
	b.mu.RLock()
	sp, ok := b.sources[source]
	b.mu.RUnlock()
	if ok {
		return sp
	}

	// Enforce maxSources outside the write lock to avoid blocking Publish.
	b.mu.RLock()
	n := len(b.sources)
	b.mu.RUnlock()
	if n >= b.maxSources {
		// Fall back to the shared "other" bucket instead of leaking a queue.
		if source != "other" {
			return b.GetOrCreateSource("other")
		}
		// "other" itself is full: return the existing one (must exist) or fail open.
		b.mu.RLock()
		defer b.mu.RUnlock()
		if sp, ok := b.sources["other"]; ok {
			return sp
		}
		// Extremely early race: create inline.
	}

	queue := NewAsyncPublisherWithOptions(b.inner, b.bufferPerSource, b.logger, b.options)

	b.mu.Lock()
	defer b.mu.Unlock()

	if sp, ok = b.sources[source]; ok {
		_ = queue.Close()
		return sp
	}
	if len(b.sources) >= b.maxSources && source != "other" {
		_ = queue.Close()
		if sp, ok := b.sources["other"]; ok {
			return sp
		}
		// Still allow creation of "other" as the last bucket.
		source = "other"
		if sp, ok := b.sources[source]; ok {
			return sp
		}
	}

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
	// Single enqueue attempt: AsyncPublisher.Publish already enforces
	// hard/soft backpressure atomically. The old double Snapshot() (here +
	// inside) cost 2-3 mutex takes per webhook.
	err := sp.queue.Publish(ctx, sp.source, body, headers)
	if err != nil {
		if b.options.Hooks.OnPublish != nil {
			b.options.Hooks.OnPublish(safeContext(ctx), sp.source, "failed", sp.queue.Snapshot())
		}
		return err
	}

	if b.options.Hooks.OnPublish != nil {
		// Emit "queued" without a second Snapshot when backpressure is off.
		b.options.Hooks.OnPublish(safeContext(ctx), sp.source, "queued", sp.queue.Snapshot())
	}
	return nil
}

func (b *BulkheadPublisher) Close() error {
	// Snapshot under RLock, close outside the lock so Publish/GetOrCreateSource
	// aren't blocked for the full drain duration.
	b.mu.RLock()
	queues := make([]*SourcePublisher, 0, len(b.sources))
	for _, sp := range b.sources {
		queues = append(queues, sp)
	}
	b.mu.RUnlock()

	var errs []error
	for _, sp := range queues {
		if err := sp.queue.Close(); err != nil {
			errs = append(errs, fmt.Errorf("source %s: %w", sp.source, err))
			b.logger.Error("bulkhead source queue close failed", "source", sp.source, "error", err)
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
	// Cache aggregate for 5s so Prometheus scrapes don't pay O(N) each time.
	if v := b.cachedAgg.Load(); v != nil {
		if snap, ok := v.(PublisherSnapshot); ok {
			if time.Since(time.Unix(0, b.cachedAt.Load())) < 5*time.Second {
				return snap
			}
		}
	}
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
	b.cachedAgg.Store(aggregate)
	b.cachedAt.Store(time.Now().UnixNano())
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
		return fmt.Errorf("inner publisher health check: %w", err)
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
