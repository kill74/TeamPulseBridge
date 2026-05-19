package failstore

import (
	"context"
	"fmt"
	"log/slog"

	"teampulsebridge/services/ingestion-gateway/internal/platform/resilience"
)

type CircuitBreakerStore struct {
	wrapped Store
	breaker *resilience.CircuitBreaker
	logger  *slog.Logger
}

func NewCircuitBreakerStore(wrapped Store, breaker *resilience.CircuitBreaker, logger *slog.Logger) *CircuitBreakerStore {
	return &CircuitBreakerStore{
		wrapped: wrapped,
		breaker: breaker,
		logger:  logger,
	}
}

func (s *CircuitBreakerStore) Save(ctx context.Context, in SaveInput) (FailedEvent, error) {
	result, err := resilience.Execute(ctx, s.breaker, func(ctx context.Context) (FailedEvent, error) {
		return s.wrapped.Save(ctx, in)
	})
	if err != nil {
		return FailedEvent{}, fmt.Errorf("circuit breaker save: %w", err)
	}
	return result, nil
}

func (s *CircuitBreakerStore) GetByID(ctx context.Context, eventID string) (FailedEvent, error) {
	return s.wrapped.GetByID(ctx, eventID)
}

func (s *CircuitBreakerStore) ListRecent(ctx context.Context, limit int) ([]FailedEvent, error) {
	return s.wrapped.ListRecent(ctx, limit)
}

func (s *CircuitBreakerStore) Delete(ctx context.Context, eventID string) error {
	_, err := resilience.Execute(ctx, s.breaker, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.wrapped.Delete(ctx, eventID)
	})
	if err != nil {
		return fmt.Errorf("circuit breaker delete: %w", err)
	}
	return nil
}

func (s *CircuitBreakerStore) UpdateRetryCount(ctx context.Context, eventID string, retryCount int) error {
	_, err := resilience.Execute(ctx, s.breaker, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.wrapped.UpdateRetryCount(ctx, eventID, retryCount)
	})
	if err != nil {
		return fmt.Errorf("circuit breaker update retry count: %w", err)
	}
	return nil
}


