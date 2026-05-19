package queue

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"sync"
	"time"
)

var ErrQueueFull = errors.New("publish queue is full")
var ErrQueueClosed = errors.New("publish queue is closed")
var ErrQueueThrottled = errors.New("publish queue is throttled")

type queuedEvent struct {
	source    string
	body      []byte
	headers   map[string]string
	createdAt time.Time
	deadline  time.Time
}

type AsyncPublisherOptions struct {
	Backpressure BackpressureConfig
	Hooks        AsyncPublisherHooks
	WorkerCount  int
}

type BackpressureConfig struct {
	Enabled               bool
	SoftLimitRatio        float64
	HardLimitRatio        float64
	FailureRatioThreshold float64
	FailureWindow         int
	MinSamples            int
}

type AsyncPublisherHooks struct {
	OnBackpressure   func(ctx context.Context, source, action string, snapshot PublisherSnapshot)
	OnPublish        func(ctx context.Context, source, result string, snapshot PublisherSnapshot)
	OnDLQ            func(ctx context.Context, source string, body []byte, headers map[string]string, err error)
	OnPublishLatency func(ctx context.Context, source string, latencySec float64, failed bool)
}

type publishWindow struct {
	results  []bool
	next     int
	count    int
	failures int
}

type AsyncPublisher struct {
	inner        Publisher
	logger       *slog.Logger
	ch           chan queuedEvent
	wg           sync.WaitGroup
	once         sync.Once
	mu           sync.RWMutex
	closed       bool
	options      AsyncPublisherOptions
	statsMu      sync.Mutex
	publishStats publishWindow
}

func NewAsyncPublisher(inner Publisher, buffer int, logger *slog.Logger) *AsyncPublisher {
	return NewAsyncPublisherWithOptions(inner, buffer, logger, AsyncPublisherOptions{})
}

func NewAsyncPublisherWithOptions(inner Publisher, buffer int, logger *slog.Logger, options AsyncPublisherOptions) *AsyncPublisher {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	options = options.withDefaults()
	p := &AsyncPublisher{
		inner:        inner,
		logger:       logger,
		ch:           make(chan queuedEvent, buffer),
		options:      options,
		publishStats: newPublishWindow(options.Backpressure.FailureWindow),
	}
	p.wg.Add(options.WorkerCount)
	for workerID := 1; workerID <= options.WorkerCount; workerID++ {
		go p.run(workerID)
	}
	return p
}

func (p *AsyncPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	e := queuedEvent{
		source:    source,
		body:      append([]byte(nil), body...),
		headers:   cloneHeaders(headers),
		createdAt: time.Now(),
	}
	if deadline, ok := ctx.Deadline(); ok {
		e.deadline = deadline
	}
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return ErrQueueClosed
	}
	snapshot := p.Snapshot()
	p.mu.RUnlock()
	if p.isHardLimited(snapshot) {
		p.emitBackpressure(ctx, source, "full", snapshot)
		return ErrQueueFull
	}
	if p.shouldThrottle(snapshot) {
		p.emitBackpressure(ctx, source, "throttled", snapshot)
		return ErrQueueThrottled
	}
	select {
	case p.ch <- e:
		return nil
	default:
		p.emitBackpressure(ctx, source, "full", p.Snapshot())
		return ErrQueueFull
	}
}

func (p *AsyncPublisher) Close() error {
	p.once.Do(func() {
		p.mu.Lock()
		p.closed = true
		close(p.ch)
		p.mu.Unlock()
	})
	p.wg.Wait()
	return nil
}

func (p *AsyncPublisher) HealthCheck(_ context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return errors.New("publisher is closed")
	}
	snapshot := p.Snapshot()
	if snapshot.UsageRatio >= p.options.Backpressure.HardLimitRatio {
		return fmt.Errorf("queue buffer usage critical: %.2f%%", snapshot.UsageRatio*100)
	}
	if snapshot.FailureRatio > p.options.Backpressure.FailureRatioThreshold && snapshot.RecentSamples >= p.options.Backpressure.MinSamples {
		return fmt.Errorf("queue failure ratio high: %.2f%%", snapshot.FailureRatio*100)
	}
	return nil
}

func (p *AsyncPublisher) run(workerID int) {
	defer p.wg.Done()
	for e := range p.ch {
		func() {
			defer func() {
				if r := recover(); r != nil {
					p.logger.Error("async publisher recovered from event panic", "worker_id", workerID, "source", e.source, "panic", r)
				}
			}()
			p.processQueuedEvent(workerID, e)
		}()
	}
}

func (p *AsyncPublisher) processQueuedEvent(workerID int, e queuedEvent) {
	ctx := context.Background()
	if !e.deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, e.deadline)
		defer cancel()
	}
	publishCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	err := p.safePublish(publishCtx, workerID, e)
	latency := time.Since(start).Seconds()
	if p.options.Hooks.OnPublishLatency != nil {
		p.options.Hooks.OnPublishLatency(publishCtx, e.source, latency, err != nil)
	}
	snapshot := p.recordPublishResult(err == nil)
	if err != nil {
		p.logger.Error(
			"failed to publish queued event",
			"worker_id", workerID,
			"source", e.source,
			"error", err,
			"queue_usage_ratio", snapshot.UsageRatio,
			"failure_ratio", snapshot.FailureRatio,
		)
		if p.options.Hooks.OnDLQ != nil {
			p.options.Hooks.OnDLQ(publishCtx, e.source, e.body, e.headers, err)
		}
	}
	p.emitPublish(publishCtx, e.source, resultLabel(err), snapshot)
}

func (p *AsyncPublisher) safePublish(ctx context.Context, workerID int, e queuedEvent) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("publish panic on worker %d: %v", workerID, r)
		}
	}()
	if err := p.inner.Publish(ctx, e.source, e.body, e.headers); err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	return nil
}

func (p *AsyncPublisher) Snapshot() PublisherSnapshot {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	return p.snapshotLocked()
}

func (p *AsyncPublisher) isHardLimited(snapshot PublisherSnapshot) bool {
	if !p.options.Backpressure.Enabled {
		return false
	}
	return snapshot.UsageRatio >= p.options.Backpressure.HardLimitRatio
}

func (p *AsyncPublisher) shouldThrottle(snapshot PublisherSnapshot) bool {
	bp := p.options.Backpressure
	if !bp.Enabled || snapshot.Capacity == 0 {
		return false
	}
	if snapshot.UsageRatio <= bp.SoftLimitRatio {
		return false
	}
	if snapshot.RecentSamples < bp.MinSamples || snapshot.FailureRatio < bp.FailureRatioThreshold {
		return false
	}
	usagePressure := normalizedPressure(snapshot.UsageRatio, bp.SoftLimitRatio, bp.HardLimitRatio)
	failurePressure := normalizedPressure(snapshot.FailureRatio, bp.FailureRatioThreshold, 1)
	return usagePressure+failurePressure >= 1
}

func (p *AsyncPublisher) recordPublishResult(success bool) PublisherSnapshot {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	p.publishStats.record(success)
	return p.snapshotLocked()
}

func (p *AsyncPublisher) snapshotLocked() PublisherSnapshot {
	capacity := cap(p.ch)
	depth := len(p.ch)
	usageRatio := 0.0
	if capacity > 0 {
		usageRatio = float64(depth) / float64(capacity)
	}
	return PublisherSnapshot{
		Depth:         depth,
		Capacity:      capacity,
		UsageRatio:    clampRatio(usageRatio),
		FailureRatio:  p.publishStats.failureRatio(),
		RecentSamples: p.publishStats.count,
	}
}

func (p *AsyncPublisher) emitBackpressure(ctx context.Context, source, action string, snapshot PublisherSnapshot) {
	if p.options.Hooks.OnBackpressure != nil {
		p.options.Hooks.OnBackpressure(safeContext(ctx), source, action, snapshot)
	}
}

func (p *AsyncPublisher) emitPublish(ctx context.Context, source, result string, snapshot PublisherSnapshot) {
	if p.options.Hooks.OnPublish != nil {
		p.options.Hooks.OnPublish(safeContext(ctx), source, result, snapshot)
	}
}

func (o AsyncPublisherOptions) withDefaults() AsyncPublisherOptions {
	if o.WorkerCount <= 0 {
		o.WorkerCount = 1
	}
	if o.Backpressure.SoftLimitRatio <= 0 {
		o.Backpressure.SoftLimitRatio = 0.70
	}
	if o.Backpressure.HardLimitRatio <= 0 {
		o.Backpressure.HardLimitRatio = 0.90
	}
	if o.Backpressure.HardLimitRatio <= o.Backpressure.SoftLimitRatio {
		o.Backpressure.HardLimitRatio = math.Min(1, o.Backpressure.SoftLimitRatio+0.10)
	}
	if o.Backpressure.FailureRatioThreshold <= 0 {
		o.Backpressure.FailureRatioThreshold = 0.15
	}
	if o.Backpressure.FailureWindow <= 0 {
		o.Backpressure.FailureWindow = 100
	}
	if o.Backpressure.MinSamples <= 0 {
		o.Backpressure.MinSamples = 20
	}
	if o.Backpressure.MinSamples > o.Backpressure.FailureWindow {
		o.Backpressure.MinSamples = o.Backpressure.FailureWindow
	}
	return o
}

func newPublishWindow(size int) publishWindow {
	if size <= 0 {
		size = 1
	}
	return publishWindow{results: make([]bool, size)}
}

func (w *publishWindow) record(success bool) {
	if len(w.results) == 0 {
		return
	}
	if w.count == len(w.results) {
		if !w.results[w.next] {
			w.failures--
		}
	} else {
		w.count++
	}
	w.results[w.next] = success
	if !success {
		w.failures++
	}
	w.next = (w.next + 1) % len(w.results)
}

func (w publishWindow) failureRatio() float64 {
	if w.count == 0 {
		return 0
	}
	return float64(w.failures) / float64(w.count)
}

func normalizedPressure(value, low, high float64) float64 {
	if high <= low {
		if value >= high {
			return 1
		}
		return 0
	}
	return clampRatio((value - low) / (high - low))
}

func clampRatio(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func resultLabel(err error) string {
	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			return "circuit_breaker_open"
		}
		return "failed"
	}
	return "success"
}

func safeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func cloneHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	cloned := make(map[string]string, len(headers))
	for k, v := range headers {
		cloned[k] = v
	}
	return cloned
}
