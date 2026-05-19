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
	result, err := resilience.Execute(ctx, s.breaker, func(ctx context.Context) (FailedEvent, error) {
		return s.wrapped.GetByID(ctx, eventID)
	})
	if err != nil {
		return FailedEvent{}, fmt.Errorf("circuit breaker get by id: %w", err)
	}
	return result, nil
}

func (s *CircuitBreakerStore) ListRecent(ctx context.Context, limit int) ([]FailedEvent, error) {
	result, err := resilience.Execute(ctx, s.breaker, func(ctx context.Context) ([]FailedEvent, error) {
		return s.wrapped.ListRecent(ctx, limit)
	})
	if err != nil {
		return nil, fmt.Errorf("circuit breaker list recent: %w", err)
	}
	return result, nil
}
