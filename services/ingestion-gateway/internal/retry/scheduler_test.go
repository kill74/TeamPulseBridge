package retry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/failstore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStore struct {
	mu           sync.Mutex
	events       []failstore.FailedEvent
	listErr      error
	recentLimit  int
}

func (m *mockStore) Save(ctx context.Context, in failstore.SaveInput) (failstore.FailedEvent, error) {
	return failstore.FailedEvent{}, nil
}

func (m *mockStore) GetByID(ctx context.Context, eventID string) (failstore.FailedEvent, error) {
	return failstore.FailedEvent{}, nil
}

func (m *mockStore) ListRecent(ctx context.Context, limit int) ([]failstore.FailedEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recentLimit = limit
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.events, nil
}

type mockPublisher struct {
	mu        sync.Mutex
	publishes []publishCall
	err       error
}

type publishCall struct {
	source  string
	body    []byte
	headers map[string]string
}

func (m *mockPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishes = append(m.publishes, publishCall{
		source:  source,
		body:    body,
		headers: headers,
	})
	return m.err
}

func (m *mockPublisher) Close() error { return nil }
func (m *mockPublisher) HealthCheck(ctx context.Context) error { return nil }

func TestScheduler_BackoffCalculation(t *testing.T) {
	s := &Scheduler{}
	
	// Test basic exponential backoff without jitter (minimum possible values)
	base := 10 * time.Second
	
	// retry 0: 2^0 * 10 = 10s
	b0 := s.calculateBackoff(0, base)
	assert.GreaterOrEqual(t, b0, 10*time.Second)
	assert.LessOrEqual(t, b0, 20*time.Second) // 10s + 10s max jitter
	
	// retry 1: 2^1 * 10 = 20s
	b1 := s.calculateBackoff(1, base)
	assert.GreaterOrEqual(t, b1, 20*time.Second)
	assert.LessOrEqual(t, b1, 30*time.Second) // 20s + 10s max jitter
	
	// retry 2: 2^2 * 10 = 40s
	b2 := s.calculateBackoff(2, base)
	assert.GreaterOrEqual(t, b2, 40*time.Second)
	assert.LessOrEqual(t, b2, 50*time.Second) // 40s + 10s max jitter
	
	// Max backoff check
	bMax := s.calculateBackoff(10, base)
	assert.Equal(t, 5*time.Minute, bMax)
}

func TestScheduler_RetryLogic(t *testing.T) {
	store := &mockStore{
		events: []failstore.FailedEvent{
			{
				EventID:  "event-1",
				Source:   "github",
				Body:     []byte(`{"id":1}`),
				FailedAt: time.Now().Add(-1 * time.Hour), // Long ago, should retry
			},
		},
	}
	pub := &mockPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	
	onRetryCalled := make(chan struct{}, 1)
	s := NewScheduler(store, pub, logger, SchedulerOptions{
		MaxRetries: 3,
		Interval:   100 * time.Millisecond,
		OnRetry: func(ctx context.Context, source string, success bool, attempt int) {
			onRetryCalled <- struct{}{}
		},
	})
	
	// Start the scheduler
	s.Start()
	
	// Wait for retry to be attempted
	select {
	case <-onRetryCalled:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for retry")
	}
	
	s.Stop()
	
	pub.mu.Lock()
	defer pub.mu.Unlock()
	require.Len(t, pub.publishes, 1)
	assert.Equal(t, "github", pub.publishes[0].source)
	assert.Equal(t, "1", pub.publishes[0].headers["X-Retry-Count"])
}

func TestScheduler_MaxRetries(t *testing.T) {
	store := &mockStore{
		events: []failstore.FailedEvent{
			{
				EventID:  "event-1",
				Source:   "github",
				Body:     []byte(`{"id":1}`),
				FailedAt: time.Now().Add(-1 * time.Hour),
			},
		},
	}
	pub := &mockPublisher{
		err: errors.New("publish failed"),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	
	retryCount := 0
	var mu sync.Mutex
	done := make(chan struct{})
	
	s := NewScheduler(store, pub, logger, SchedulerOptions{
		MaxRetries: 2,
		Interval:   10 * time.Millisecond,
		OnRetry: func(ctx context.Context, source string, success bool, attempt int) {
			mu.Lock()
			retryCount++
			if retryCount == 2 {
				close(done)
			}
			mu.Unlock()
		},
	})
	
	s.Start()
	
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for max retries")
	}
	
	s.Stop()
	
	// Wait a bit to ensure no more retries happen
	time.Sleep(50 * time.Millisecond)
	
	mu.Lock()
	assert.Equal(t, 2, retryCount)
	mu.Unlock()
}

func TestScheduler_Stop(t *testing.T) {
	store := &mockStore{}
	pub := &mockPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	
	s := NewScheduler(store, pub, logger, SchedulerOptions{
		Interval: 10 * time.Millisecond,
	})
	
	s.Start()
	assert.True(t, s.running)
	
	s.Stop()
	assert.False(t, s.running)
	assert.True(t, s.stopped)
	
	// Stopping again should not panic
	assert.NotPanics(t, func() {
		s.Stop()
	})
}
