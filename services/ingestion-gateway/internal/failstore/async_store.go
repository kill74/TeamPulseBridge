package failstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrAsyncDropped is returned when the async queue is full and the record
// is dropped fail-open. Callers must treat this as a real persistence
// failure (log + metrics), not success.
var ErrAsyncDropped = errors.New("async store queue full, record dropped")

// AsyncStore wraps a Store with a non-blocking enqueue channel so webhook
// error paths return 429/503 immediately instead of blocking on fsync.
// On channel full it drops with a counter (fail-open) and returns the inner
// error semantics via ErrQueueFull-like behavior: Save returns nil record + nil
// error? No — it returns the dropped error so callers can still log, but
// without stalling. See docs/performance.md.
type AsyncStore struct {
	inner     Store
	ch        chan asyncSaveReq
	dropped   atomic.Int64
	closed    atomic.Bool
	closeMu   sync.Mutex
	closeCh   chan struct{}
	closeOnce sync.Once
}

type asyncSaveReq struct {
	ctx    context.Context
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
	a := &AsyncStore{inner: inner, ch: make(chan asyncSaveReq, buffer), closeCh: make(chan struct{})}
	go a.run()
	return a
}

// Close drains buffered records then stops the background writer.
// Saves after Close fall back to synchronous inner.Save.
func (a *AsyncStore) Close() {
	a.closeOnce.Do(func() {
		a.closed.Store(true)
		close(a.closeCh)
	})
}

func (a *AsyncStore) persist(req asyncSaveReq) {
	// Detach from request cancellation but preserve values (trace IDs).
	persistCtx := context.Background()
	if req.ctx != nil {
		persistCtx = context.WithoutCancel(req.ctx)
	}
	rec, err := a.inner.Save(persistCtx, req.in)
	// Non-blocking reply; request may have gone away.
	select {
	case req.result <- asyncSaveResult{record: rec, err: err}:
	default:
	}
}

func (a *AsyncStore) run() {
	for {
		select {
		case req, ok := <-a.ch:
			if !ok {
				return
			}
			a.persist(req)
		case <-a.closeCh:
			// Drain buffered records, then exit. No new sends happen after
			// Close (Save falls back to sync), so ranging is safe.
			for {
				select {
				case req := <-a.ch:
					a.persist(req)
				default:
					return
				}
			}
		}
	}
}

// Save enqueues with a 500ms budget; on full channel it returns
// ErrAsyncDropped so callers surface the loss via logs/metrics instead of
// silently reporting success.
func (a *AsyncStore) Save(ctx context.Context, in SaveInput) (FailedEvent, error) {
	if a.closed.Load() {
		return a.inner.Save(ctx, in)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resCh := make(chan asyncSaveResult, 1)
	req := asyncSaveReq{ctx: ctx, in: in, result: resCh}
	select {
	case a.ch <- req:
		select {
		case res := <-resCh:
			return res.record, res.err
		case <-ctx.Done():
			// Caller gave up; worker will still persist in background
			// via WithoutCancel. Report cancellation accurately.
			return FailedEvent{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			// Worker is slow but will still persist in background;
			// surface timeout so callers don't misreport success.
			return FailedEvent{}, context.DeadlineExceeded
		}
	default:
		a.dropped.Add(1)
		return FailedEvent{}, fmt.Errorf("%w: failed-event %s/%s", ErrAsyncDropped, in.Source, in.EventID)
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
