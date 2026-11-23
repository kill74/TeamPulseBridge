package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)
	if !cb.Allow() {
		t.Fatal("closed breaker should allow")
	}
	cb.RecordFailure()
	cb.RecordFailure()
	if state := cb.State(); state != "closed" {
		t.Fatalf("state = %q, want closed", state)
	}
	cb.RecordFailure()
	if state := cb.State(); state != "open" {
		t.Fatalf("state = %q, want open", state)
	}
	if cb.Allow() {
		t.Fatal("open breaker should reject")
	}
}

func TestCircuitBreakerHalfOpenProbeAndRecovery(t *testing.T) {
	cb := NewCircuitBreaker(1, 20*time.Millisecond)
	cb.RecordFailure()
	if state := cb.State(); state != "open" {
		t.Fatalf("state = %q, want open", state)
	}
	time.Sleep(50 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("breaker should allow one half-open probe after recovery delay")
	}
	if cb.Allow() {
		t.Fatal("second probe while one is in flight should be rejected")
	}
	cb.RecordSuccess()
	if state := cb.State(); state != "closed" {
		t.Fatalf("state = %q, want closed after probe success", state)
	}
	if !cb.Allow() {
		t.Fatal("closed breaker should allow after recovery")
	}
}

func TestCircuitBreakerSuccessResetsFailures(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Minute)
	cb.RecordFailure()
	cb.RecordSuccess()
	cb.RecordFailure()
	if state := cb.State(); state != "closed" {
		t.Fatalf("state = %q, want closed (failure count reset by success)", state)
	}
}

func TestExecuteRejectsWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Minute)
	cb.RecordFailure()
	called := false
	_, err := Execute(context.Background(), cb, func(ctx context.Context) (string, error) {
		called = true
		return "ok", nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if called {
		t.Fatal("function must not run while breaker is open")
	}
}

func TestExecuteRecordsSuccessAndFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Minute)
	if _, err := Execute(context.Background(), cb, func(ctx context.Context) (string, error) {
		return "ok", nil
	}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	wantErr := errors.New("boom")
	if _, err := Execute(context.Background(), cb, func(ctx context.Context) (string, error) {
		return "", wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped error, got %v", err)
	}
	if state := cb.State(); state != "closed" {
		t.Fatalf("state = %q, want closed (below threshold)", state)
	}
}

func TestNewCircuitBreakerAppliesDefaults(t *testing.T) {
	cb := NewCircuitBreaker(0, 0)
	if !cb.Allow() {
		t.Fatal("default breaker should allow")
	}
	if err := cb.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
