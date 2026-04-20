package queue

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
)

var ErrQueueFull = errors.New("publish queue is full")
var ErrQueueClosed = errors.New("publish queue is closed")

type queuedEvent struct {
	ctx     context.Context
	source  string
	body    []byte
	headers map[string]string
}

type AsyncPublisher struct {
	inner  Publisher
	logger *slog.Logger
	ch     chan queuedEvent
	wg     sync.WaitGroup
	once   sync.Once
	mu     sync.RWMutex
	closed bool
}

func NewAsyncPublisher(inner Publisher, buffer int, logger *slog.Logger) *AsyncPublisher {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	p := &AsyncPublisher{
		inner:  inner,
		logger: logger,
		ch:     make(chan queuedEvent, buffer),
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
	select {
	case p.ch <- e:
		return nil
	default:
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
		if err := p.inner.Publish(e.ctx, e.source, e.body, e.headers); err != nil {
			p.logger.Error("failed to publish queued event", "source", e.source, "error", err)
		}
	}
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
