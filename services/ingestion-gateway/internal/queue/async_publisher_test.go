package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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

func (p *blockingPublisher) HealthCheck(_ context.Context) error {
	return nil
}

type contextCapturePublisher struct {
	started chan struct{}
	release chan struct{}
	ctxErr  error
}

func (p *contextCapturePublisher) Publish(ctx context.Context, _ string, _ []byte, _ map[string]string) error {
	close(p.started)
	<-p.release
	p.ctxErr = ctx.Err()
	return nil
}

func (p *contextCapturePublisher) Close() error { return nil }

func (p *contextCapturePublisher) HealthCheck(_ context.Context) error { return nil }

type concurrentTrackingPublisher struct {
	release chan struct{}
	active  atomic.Int64
	maxSeen atomic.Int64
}

func (p *concurrentTrackingPublisher) Publish(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	active := p.active.Add(1)
	for {
		maxSeen := p.maxSeen.Load()
		if active <= maxSeen || p.maxSeen.CompareAndSwap(maxSeen, active) {
			break
		}
	}
	<-p.release
	p.active.Add(-1)
	return nil
}

func (p *concurrentTrackingPublisher) Close() error { return nil }

func (p *concurrentTrackingPublisher) HealthCheck(_ context.Context) error { return nil }

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

func (p *scriptedPublisher) HealthCheck(_ context.Context) error {
	return p.err
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

func TestAsyncPublisherUsesPublishContextIndependentFromCanceledRequest(t *testing.T) {
	inner := &contextCapturePublisher{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	p := NewAsyncPublisherWithOptions(inner, 1, nil, AsyncPublisherOptions{WorkerCount: 1})

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, p.Publish(ctx, "github", []byte(`{}`), nil))
	<-inner.started
	cancel()
	close(inner.release)
	require.NoError(t, p.Close())
	require.NoError(t, inner.ctxErr)
}

func TestAsyncPublisherProcessesEventsWithMultipleWorkers(t *testing.T) {
	inner := &concurrentTrackingPublisher{release: make(chan struct{})}
	p := NewAsyncPublisherWithOptions(inner, 3, nil, AsyncPublisherOptions{WorkerCount: 3})

	for i := 0; i < 3; i++ {
		require.NoError(t, p.Publish(context.Background(), "github", []byte(`{}`), nil))
	}

	require.Eventually(t, func() bool {
		return inner.maxSeen.Load() == 3
	}, time.Second, 10*time.Millisecond)

	close(inner.release)
	require.NoError(t, p.Close())
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
