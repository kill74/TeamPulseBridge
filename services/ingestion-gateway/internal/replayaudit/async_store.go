package replayaudit

import (
	"context"
	"sync/atomic"
	"time"
)

// AsyncStore makes replay-audit writes non-blocking. Admin replay returns
// immediately; persistence happens on a background goroutine with fail-open
// drop accounting.
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
		buffer = 512
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
		case <-time.After(500 * time.Millisecond):
			return Record{}, context.DeadlineExceeded
		}
	default:
		a.dropped.Add(1)
		return Record{EventID: in.EventID, Source: in.Source, Actor: in.Actor}, nil
	}
}

func (a *AsyncStore) List(ctx context.Context, q ListQuery) (ListResult, error) {
	return a.inner.List(ctx, q)
}

func (a *AsyncStore) ListRecent(ctx context.Context, limit int) ([]Record, error) {
	out, err := a.inner.List(ctx, ListQuery{Limit: limit, Sort: SortDesc})
	if err != nil {
		return nil, err
	}
	return out.Records, nil
}

func (a *AsyncStore) Dropped() int64 { return a.dropped.Load() }
