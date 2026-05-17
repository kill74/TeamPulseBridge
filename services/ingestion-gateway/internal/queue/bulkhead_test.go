package queue_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

func TestBulkheadPublisher(t *testing.T) {
	logger := slog.Default()
	mp := &mockPublisher{}

	opts := queue.AsyncPublisherOptions{
		Backpressure: queue.BackpressureConfig{
			Enabled:               true,
			SoftLimitRatio:        0.5,
			HardLimitRatio:        0.9,
			FailureRatioThreshold: 0.5,
			FailureWindow:         60,
			MinSamples:            5,
		},
	}

	bp := queue.NewBulkheadPublisher(mp, 10, logger, opts)
	defer func() { _ = bp.Close() }()

	ctx := context.Background()

	// Publish successfully
	err := bp.Publish(ctx, "github", []byte("data1"), nil)
	require.NoError(t, err)

	err = bp.Publish(ctx, "slack", []byte("data2"), nil)
	require.NoError(t, err)

	snapshots := bp.SourceSnapshots()
	assert.Contains(t, snapshots, "github")
	assert.Contains(t, snapshots, "slack")
}

type countingPublisher struct {
	calls atomic.Int64
}

func (p *countingPublisher) Publish(context.Context, string, []byte, map[string]string) error {
	p.calls.Add(1)
	return nil
}

func (p *countingPublisher) Close() error { return nil }

func (p *countingPublisher) HealthCheck(context.Context) error { return nil }

func TestBulkheadPublisherReleasesSourceCapacityAfterAsyncPublish(t *testing.T) {
	publisher := &countingPublisher{}
	bp := queue.NewBulkheadPublisher(publisher, 1, slog.Default(), queue.AsyncPublisherOptions{
		Backpressure: queue.BackpressureConfig{
			Enabled:        true,
			SoftLimitRatio: 0.5,
			HardLimitRatio: 1,
		},
	})
	defer func() { _ = bp.Close() }()

	ctx := context.Background()
	require.NoError(t, bp.Publish(ctx, "github", []byte("data1"), nil))
	require.Eventually(t, func() bool {
		return publisher.calls.Load() >= 1
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, bp.Publish(ctx, "github", []byte("data2"), nil))
	require.Eventually(t, func() bool {
		return publisher.calls.Load() >= 2
	}, time.Second, 10*time.Millisecond)
}
