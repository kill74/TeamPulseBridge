package failstore

import (
	"context"
	"errors"
	"testing"

	"teampulsebridge/services/ingestion-gateway/internal/platform/resilience"
)

// TestAsyncStoreUnavailableInner verifies that a disabled (nil or typed-nil)
// inner store never panics, including after Close.
func TestAsyncStoreUnavailableInner(t *testing.T) {
	var pgStore *PostgresStore
	cases := map[string]Store{
		"nil interface": nil,
		"typed nil":     pgStore,
	}
	for name, inner := range cases {
		t.Run(name, func(t *testing.T) {
			for _, closed := range []bool{false, true} {
				a := NewAsyncStore(inner, 1)
				if closed {
					a.Close()
				}
				ctx := context.Background()

				if _, err := a.Save(ctx, SaveInput{EventID: "e1"}); !errors.Is(err, ErrStoreUnavailable) {
					t.Errorf("Save: expected ErrStoreUnavailable, got %v", err)
				}
				if _, err := a.GetByID(ctx, "e1"); !errors.Is(err, ErrStoreUnavailable) {
					t.Errorf("GetByID: expected ErrStoreUnavailable, got %v", err)
				}
				if _, err := a.ListRecent(ctx, 1); !errors.Is(err, ErrStoreUnavailable) {
					t.Errorf("ListRecent: expected ErrStoreUnavailable, got %v", err)
				}
				if err := a.Delete(ctx, "e1"); !errors.Is(err, ErrStoreUnavailable) {
					t.Errorf("Delete: expected ErrStoreUnavailable, got %v", err)
				}
				if err := a.UpdateRetryCount(ctx, "e1", 1); !errors.Is(err, ErrStoreUnavailable) {
					t.Errorf("UpdateRetryCount: expected ErrStoreUnavailable, got %v", err)
				}
				a.Close()
			}
		})
	}
}

func TestCircuitBreakerStoreUnavailableInner(t *testing.T) {
	var pgStore *PostgresStore
	cases := map[string]Store{
		"nil interface": nil,
		"typed nil":     pgStore,
	}
	for name, inner := range cases {
		t.Run(name, func(t *testing.T) {
			s := NewCircuitBreakerStore(inner, resilience.NewCircuitBreaker(5, 0), nil)
			ctx := context.Background()

			if _, err := s.Save(ctx, SaveInput{EventID: "e1"}); !errors.Is(err, ErrStoreUnavailable) {
				t.Errorf("Save: expected ErrStoreUnavailable, got %v", err)
			}
			if _, err := s.GetByID(ctx, "e1"); !errors.Is(err, ErrStoreUnavailable) {
				t.Errorf("GetByID: expected ErrStoreUnavailable, got %v", err)
			}
			if _, err := s.ListRecent(ctx, 1); !errors.Is(err, ErrStoreUnavailable) {
				t.Errorf("ListRecent: expected ErrStoreUnavailable, got %v", err)
			}
			if err := s.Delete(ctx, "e1"); !errors.Is(err, ErrStoreUnavailable) {
				t.Errorf("Delete: expected ErrStoreUnavailable, got %v", err)
			}
			if err := s.UpdateRetryCount(ctx, "e1", 1); !errors.Is(err, ErrStoreUnavailable) {
				t.Errorf("UpdateRetryCount: expected ErrStoreUnavailable, got %v", err)
			}
		})
	}
}
