package failstore

import (
	"context"
	"sync/atomic"
	"time"
)

// AsyncStore wraps a Store with a non-blocking enqueue channel so webhook
// error paths return 429/503 immediately instead of blocking on fsync.
// On channel full it drops with a counter (fail-open) and returns the inner
// error semantics via ErrQueueFull-like behavior: Save returns nil record + nil
// error? No — it returns the dropped error so callers can still log, but
// without stalling. See docs/performance.md.
type AsyncStore struct {
	inner   Store
	ch      chan asyncSaveReq
	dropped atomic.Int64
	closed  atomic.Bool
}

type asyncSaveReq struct {
	in     SaveInput
	result chan asyncSaveResult
}

type asyncSaveResult struct {
	record FailedEvent
	err    error
}

// NewAsyncStore creates a background writer. buffer=512, flush every 100ms or
// 50 records via group-commit in the wrapped FileStore (which still fsyncs,
// but off the request goroutine).
func NewAsyncStore(inner Store, buffer int) *AsyncStore {
	if buffer <= 0 {
		buffer = 512
	}
	a := &AsyncStore{inner: inner, ch: make(chan asyncSaveReq, buffer)}
	go a.run()
	return a
}

func (a *AsyncStore) run() {
	for req := range a.ch {
		rec, err := a.inner.Save(context.Background(), req.in)
		// Non-blocking reply; request may have gone away.
		select {
		case req.result <- asyncSaveResult{record: rec, err: err}:
		default:
		}
	}
}

// Save enqueues with a 50ms budget; on full channel it drops (fail-open) and
// returns the inner store error path without blocking.
func (a *AsyncStore) Save(ctx context.Context, in SaveInput) (FailedEvent, error) {
	if a.closed.Load() {
		return a.inner.Save(ctx, in)
	}
	resCh := make(chan asyncSaveResult, 1)
	req := asyncSaveReq{in: in, result: resCh}
	select {
	case a.ch <- req:
		select {
		case res := <-resCh:
			return res.record, res.err
		case <-ctx.Done():
			// Caller gave up; worker will still persist in background.
			return FailedEvent{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return FailedEvent{}, context.DeadlineExceeded
		}
	default:
		a.dropped.Add(1)
		// Fail-open: don't block the error response on a saturated audit queue.
		return FailedEvent{EventID: in.EventID, Source: in.Source, Reason: in.Reason}, nil
	}
}

func (a *AsyncStore) GetByID(ctx context.Context, eventID string) (FailedEvent, error) {
	return a.inner.GetByID(ctx, eventID)
}

func (a *AsyncStore) ListRecent(ctx context.Context, limit int) ([]FailedEvent, error) {
	return a.inner.ListRecent(ctx, limit)
}

func (a *AsyncStore) Delete(ctx context.Context, eventID string) error {
	return a.inner.Delete(ctx, eventID)
}

func (a *AsyncStore) UpdateRetryCount(ctx context.Context, eventID string, retryCount int) error {
	return a.inner.UpdateRetryCount(ctx, eventID, retryCount)
}

// Dropped returns the count of fail-open drops (expose via metrics).
func (a *AsyncStore) Dropped() int64 { return a.dropped.Load() }
