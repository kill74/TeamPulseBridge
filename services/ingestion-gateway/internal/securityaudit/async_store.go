package securityaudit

import (
	"context"
	"sync/atomic"
	"time"
)

// AsyncStore makes security-reject logging non-blocking. Every 401/405/429
// previously did file I/O (MkdirAll+Write+Sync+prune scan) on the request
// goroutine; now it enqueues with fail-open drops.
type AsyncStore struct {
	inner   Store
	ch      chan asyncReq
	dropped atomic.Int64
	closed  atomic.Bool
}

type asyncReq struct {
	in     SaveInput
	result chan asyncResult
}

type asyncResult struct {
	record Record
	err    error
}

func NewAsyncStore(inner Store, buffer int) *AsyncStore {
	if buffer <= 0 {
		buffer = 1024
	}
	a := &AsyncStore{inner: inner, ch: make(chan asyncReq, buffer)}
	go a.run()
	return a
}

func (a *AsyncStore) run() {
	for req := range a.ch {
		rec, err := a.inner.Save(context.Background(), req.in)
		select {
		case req.result <- asyncResult{record: rec, err: err}:
		default:
		}
	}
}

func (a *AsyncStore) Save(ctx context.Context, in SaveInput) (Record, error) {
	if a.closed.Load() {
		return a.inner.Save(ctx, in)
	}
	resCh := make(chan asyncResult, 1)
	select {
	case a.ch <- asyncReq{in: in, result: resCh}:
		select {
		case res := <-resCh:
			return res.record, res.err
		case <-ctx.Done():
			return Record{}, ctx.Err()
		case <-time.After(300 * time.Millisecond):
			// Don't stall rejects; audit will still land in background.
			return Record{}, context.DeadlineExceeded
		}
	default:
		a.dropped.Add(1)
		return Record{}, nil
	}
}

func (a *AsyncStore) ListRecent(ctx context.Context, limit int) ([]Record, error) {
	return a.inner.ListRecent(ctx, limit)
}

func (a *AsyncStore) Dropped() int64 { return a.dropped.Load() }
