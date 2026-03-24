package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

var ErrQueueFull = errors.New("publish queue is full")

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
}

func NewAsyncPublisher(inner Publisher, buffer int, logger *slog.Logger) *AsyncPublisher {
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
	e := queuedEvent{ctx: ctx, source: source, body: body, headers: headers}
	select {
	case p.ch <- e:
		return nil
	default:
		return ErrQueueFull
	}
}

func (p *AsyncPublisher) Close() {
	close(p.ch)
	p.wg.Wait()
}

func (p *AsyncPublisher) run() {
	defer p.wg.Done()
	for e := range p.ch {
		if err := p.inner.Publish(e.ctx, e.source, e.body, e.headers); err != nil {
			p.logger.Error("failed to publish queued event", "source", e.source, "error", err)
		}
	}
}
