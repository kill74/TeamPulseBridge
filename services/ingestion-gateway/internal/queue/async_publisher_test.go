package queue

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockingPublisher struct {
	started chan struct{}
	release chan struct{}

	mu      sync.Mutex
	source  string
	body    []byte
	headers map[string]string
}

func (p *blockingPublisher) Publish(_ context.Context, source string, body []byte, headers map[string]string) error {
	close(p.started)
	<-p.release

	p.mu.Lock()
	defer p.mu.Unlock()
	p.source = source
	p.body = append([]byte(nil), body...)
	p.headers = cloneHeaders(headers)
	return nil
}

func (p *blockingPublisher) Close() error {
	return nil
}

func TestAsyncPublisherPublishAfterCloseReturnsErrQueueClosed(t *testing.T) {
	t.Parallel()

	p := NewAsyncPublisher(&blockingPublisher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}, 1, nil)

	require.NoError(t, p.Close())

	err := p.Publish(context.Background(), "github", []byte(`{}`), map[string]string{"X-Test": "1"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQueueClosed)
}

func TestAsyncPublisherClonesQueuedPayloads(t *testing.T) {
	t.Parallel()

	inner := &blockingPublisher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	p := NewAsyncPublisher(inner, 1, nil)

	body := []byte(`{"status":"ok"}`)
	headers := map[string]string{"X-Test": "original"}

	require.NoError(t, p.Publish(context.Background(), "github", body, headers))
	<-inner.started

	body[0] = 'x'
	headers["X-Test"] = "mutated"
	headers["X-New"] = "added"

	close(inner.release)
	require.NoError(t, p.Close())

	inner.mu.Lock()
	defer inner.mu.Unlock()
	assert.Equal(t, "github", inner.source)
	assert.Equal(t, `{"status":"ok"}`, string(inner.body))
	assert.Equal(t, map[string]string{"X-Test": "original"}, inner.headers)
}
