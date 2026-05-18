package failstore

import (
	"context"
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
	return resilience.Execute(ctx, s.breaker, func(ctx context.Context) (FailedEvent, error) {
		return s.wrapped.Save(ctx, in)
	})
}

func (s *CircuitBreakerStore) GetByID(ctx context.Context, eventID string) (FailedEvent, error) {
	return resilience.Execute(ctx, s.breaker, func(ctx context.Context) (FailedEvent, error) {
		return s.wrapped.GetByID(ctx, eventID)
	})
}

func (s *CircuitBreakerStore) ListRecent(ctx context.Context, limit int) ([]FailedEvent, error) {
	return resilience.Execute(ctx, s.breaker, func(ctx context.Context) ([]FailedEvent, error) {
		return s.wrapped.ListRecent(ctx, limit)
	})
}
