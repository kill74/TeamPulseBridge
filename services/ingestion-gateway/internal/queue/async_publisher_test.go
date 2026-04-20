package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

type scriptedPublisher struct {
	started chan struct{}
	release chan struct{}
	err     error

	once  sync.Once
	mu    sync.Mutex
	calls int
}

func (p *scriptedPublisher) Publish(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	if call == 1 {
		return p.err
	}
	p.once.Do(func() {
		close(p.started)
	})
	<-p.release
	return nil
}

func (p *scriptedPublisher) Close() error {
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

func TestAsyncPublisherAdaptiveBackpressureThrottlesUnderFailureBudgetBurn(t *testing.T) {
	t.Parallel()

	inner := &scriptedPublisher{
		started: make(chan struct{}),
		release: make(chan struct{}),
		err:     errors.New("downstream publish failed"),
	}
	p := NewAsyncPublisherWithOptions(inner, 4, nil, AsyncPublisherOptions{
		Backpressure: BackpressureConfig{
			Enabled:               true,
			SoftLimitRatio:        0.50,
			HardLimitRatio:        0.95,
			FailureRatioThreshold: 0.25,
			FailureWindow:         4,
			MinSamples:            1,
		},
	})

	require.NoError(t, p.Publish(context.Background(), "github", []byte(`{"id":1}`), nil))

	require.Eventually(t, func() bool {
		return p.Snapshot().RecentSamples == 1 && p.Snapshot().FailureRatio >= 1
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, p.Publish(context.Background(), "github", []byte(`{"id":2}`), nil))
	<-inner.started

	require.NoError(t, p.Publish(context.Background(), "github", []byte(`{"id":3}`), nil))
	require.NoError(t, p.Publish(context.Background(), "github", []byte(`{"id":4}`), nil))
	require.NoError(t, p.Publish(context.Background(), "github", []byte(`{"id":5}`), nil))

	err := p.Publish(context.Background(), "github", []byte(`{"id":6}`), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrQueueThrottled)

	close(inner.release)
	require.NoError(t, p.Close())
}
