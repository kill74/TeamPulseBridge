package queue

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sync"
)

var ErrQueueFull = errors.New("publish queue is full")
var ErrQueueClosed = errors.New("publish queue is closed")
var ErrQueueThrottled = errors.New("publish queue is throttled")

type queuedEvent struct {
	ctx     context.Context
	source  string
	body    []byte
	headers map[string]string
}

type AsyncPublisherOptions struct {
	Backpressure BackpressureConfig
	Hooks        AsyncPublisherHooks
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
	OnBackpressure func(ctx context.Context, source, action string, snapshot PublisherSnapshot)
	OnPublish      func(ctx context.Context, source, result string, snapshot PublisherSnapshot)
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
	p.wg.Add(1)
	go p.run()
	return p
}

func (p *AsyncPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	e := queuedEvent{
		ctx:     ctx,
		source:  source,
		body:    append([]byte(nil), body...),
		headers: cloneHeaders(headers),
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return ErrQueueClosed
	}
	snapshot := p.Snapshot()
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

func (p *AsyncPublisher) run() {
	defer p.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("async publisher recovered from panic", "panic", r)
		}
	}()
	for e := range p.ch {
		err := p.inner.Publish(e.ctx, e.source, e.body, e.headers)
		snapshot := p.recordPublishResult(err == nil)
		if err != nil {
			p.logger.Error(
				"failed to publish queued event",
				"source", e.source,
				"error", err,
				"queue_usage_ratio", snapshot.UsageRatio,
				"failure_ratio", snapshot.FailureRatio,
			)
		}
		p.emitPublish(e.ctx, e.source, resultLabel(err), snapshot)
	}
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
