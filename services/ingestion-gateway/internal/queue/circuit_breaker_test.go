package queue_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

type mockPublisher struct {
	err error
}

func (m *mockPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	return m.err
}

func (m *mockPublisher) Close() error {
	return nil
}

func (m *mockPublisher) HealthCheck(ctx context.Context) error {
	return nil
}

func TestCircuitBreaker(t *testing.T) {
	cb := queue.NewCircuitBreaker(3, 100*time.Millisecond)
	assert.True(t, cb.Allow())
	assert.Equal(t, "closed", cb.State())

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	assert.False(t, cb.Allow())
	assert.Equal(t, "open", cb.State())

	time.Sleep(150 * time.Millisecond)

	assert.True(t, cb.Allow())
	assert.Equal(t, "half-open", cb.State())

	cb.RecordSuccess()
	assert.Equal(t, "closed", cb.State())
}

func TestCircuitBreakerPublisher(t *testing.T) {
	logger := slog.Default()
	mp := &mockPublisher{}
	cb := queue.NewCircuitBreaker(2, 50*time.Millisecond)
	p := queue.NewCircuitBreakerPublisher(mp, cb, logger)

	// Success
	err := p.Publish(context.Background(), "src", []byte("body"), nil)
	require.NoError(t, err)

	// Failures
	mp.err = errors.New("failing")
	err = p.Publish(context.Background(), "src", []byte("body"), nil)
	require.Error(t, err)
	err = p.Publish(context.Background(), "src", []byte("body"), nil)
	require.Error(t, err)

	// Circuit open
	err = p.Publish(context.Background(), "src", []byte("body"), nil)
	require.Equal(t, queue.ErrCircuitOpen, err)

	// Wait for half-open
	time.Sleep(100 * time.Millisecond)
	mp.err = nil // Recover
	err = p.Publish(context.Background(), "src", []byte("body"), nil)
	require.NoError(t, err)
	assert.Equal(t, "closed", cb.State())
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := queue.NewCircuitBreaker(2, 50*time.Millisecond)

	// Closed -> Open
	cb.RecordFailure()
	cb.RecordFailure()
	assert.Equal(t, "open", cb.State())
	assert.False(t, cb.Allow()) // Ensure Allow returns false when open

	// Open -> Half-Open
	time.Sleep(100 * time.Millisecond)
	assert.True(t, cb.Allow()) // Transition to half-open
	assert.Equal(t, "half-open", cb.State())
	assert.True(t, cb.Allow()) // Ensure subsequent Allow returns true while half-open

	// Half-Open -> Open (Failure during half-open)
	cb.RecordFailure()
	assert.Equal(t, "open", cb.State())
	assert.False(t, cb.Allow())

	// Open -> Half-Open -> Closed
	time.Sleep(100 * time.Millisecond)
	assert.True(t, cb.Allow()) // Transition to half-open again

	cb.RecordSuccess()
	assert.Equal(t, "closed", cb.State())
	assert.True(t, cb.Allow())
}

func TestCircuitBreakerPublisher_CloseAndHealthCheck(t *testing.T) {
	logger := slog.Default()
	mp := &mockPublisher{}
	cb := queue.NewCircuitBreaker(2, 50*time.Millisecond)
	p := queue.NewCircuitBreakerPublisher(mp, cb, logger)

	err := p.Close()
	require.NoError(t, err)

	err = p.HealthCheck(context.Background())
	require.NoError(t, err)
}
