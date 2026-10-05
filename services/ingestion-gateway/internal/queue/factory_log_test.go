package queue

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"teampulsebridge/services/ingestion-gateway/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The log backend is the local/dev path: build the full runtime publisher,
// publish through it, and verify snapshot/close wiring.
func TestBuildRuntimePublisherLogBackend(t *testing.T) {
	cfg := config.Config{
		QueueBackend:                      "log",
		QueueBuffer:                       64,
		QueueWorkers:                      2,
		QueueBulkheadEnabled:              false,
		QueueBackpressureEnabled:          true,
		QueueBackpressureSoftLimitPercent: 70,
		QueueBackpressureHardLimitPercent: 90,
		QueueFailureBudgetPercent:         15,
		QueueFailureBudgetWindow:          100,
		QueueFailureBudgetMinSamples:      20,
	}
	ctx := context.Background()
	rp, err := BuildRuntimePublisher(ctx, cfg, testLogger(), AsyncPublisherOptions{})
	if err != nil {
		t.Fatalf("build runtime publisher: %v", err)
	}
	if rp.Publisher == nil {
		t.Fatal("expected non-nil publisher")
	}
	if err := rp.Publisher.Publish(ctx, "github", []byte(`{"action":"opened"}`), map[string]string{"X-Test": "1"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	snap := rp.Snapshot()
	if snap.Capacity == 0 {
		t.Fatal("expected snapshot with capacity")
	}
	if rp.SourceSnapshots() != nil {
		t.Fatal("expected nil source snapshots without bulkhead")
	}
	if err := rp.Publisher.HealthCheck(ctx); err != nil {
		t.Fatalf("health check: %v", err)
	}
	if err := rp.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestLogPublisherRoundTrip(t *testing.T) {
	p := NewLogPublisher(testLogger())
	ctx := context.Background()
	if err := p.Publish(ctx, "slack", []byte(`{}`), nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := p.HealthCheck(ctx); err != nil {
		t.Fatalf("health check: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
