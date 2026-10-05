package replayaudit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrAsyncDropped is returned when the async queue is full and the record
// is dropped fail-open.
var ErrAsyncDropped = errors.New("async store queue full, record dropped")

// AsyncStore makes replay-audit writes non-blocking. Admin replay returns
// immediately; persistence happens on a background goroutine with fail-open
// drop accounting.
type AsyncStore struct {
	inner     Store
	ch        chan asyncReq
	dropped   atomic.Int64
	closed    atomic.Bool
	closeCh   chan struct{}
	closeOnce sync.Once
}

type asyncReq struct {
	ctx    context.Context
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
	a := &AsyncStore{inner: inner, ch: make(chan asyncReq, buffer), closeCh: make(chan struct{})}
	go a.run()
	return a
}

// Close drains buffered records then stops the background writer.
func (a *AsyncStore) Close() {
	a.closeOnce.Do(func() {
		a.closed.Store(true)
		close(a.closeCh)
	})
}

func (a *AsyncStore) persist(req asyncReq) {
	persistCtx := context.Background()
	if req.ctx != nil {
		persistCtx = context.WithoutCancel(req.ctx)
	}
	rec, err := a.inner.Save(persistCtx, req.in)
	select {
	case req.result <- asyncResult{record: rec, err: err}:
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

func (a *AsyncStore) Save(ctx context.Context, in SaveInput) (Record, error) {
	if a.closed.Load() {
		return a.inner.Save(ctx, in)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resCh := make(chan asyncResult, 1)
	select {
	case a.ch <- asyncReq{ctx: ctx, in: in, result: resCh}:
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
		return Record{}, fmt.Errorf("%w: replay-audit %s/%s", ErrAsyncDropped, in.Source, in.EventID)
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
