package queue

import (
	"context"
	"fmt"
	"log/slog"

	"teampulsebridge/services/ingestion-gateway/internal/platform/resilience"
)

var ErrCircuitOpen = resilience.ErrCircuitOpen

type CircuitBreakerPublisher struct {
	wrapped Publisher
	breaker *resilience.CircuitBreaker
	logger  *slog.Logger
}

func NewCircuitBreakerPublisher(wrapped Publisher, breaker *resilience.CircuitBreaker, logger *slog.Logger) *CircuitBreakerPublisher {
	return &CircuitBreakerPublisher{
		wrapped: wrapped,
		breaker: breaker,
		logger:  logger,
	}
}

func (p *CircuitBreakerPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	if !p.breaker.Allow() {
		p.logger.Warn("circuit breaker open, rejecting publish", "source", source)
		return ErrCircuitOpen
	}
	err := p.wrapped.Publish(ctx, source, body, headers)
	if err != nil {
		p.breaker.RecordFailure()
		return fmt.Errorf("circuit breaker wrapped publish: %w", err)
	}
	p.breaker.RecordSuccess()
	return nil
}

func (p *CircuitBreakerPublisher) Close() error {
	_ = p.breaker.Close()
	if err := p.wrapped.Close(); err != nil {
		return fmt.Errorf("circuit breaker wrapped close: %w", err)
	}
	return nil
}

func (p *CircuitBreakerPublisher) HealthCheck(ctx context.Context) error {
	if err := p.wrapped.HealthCheck(ctx); err != nil {
		return fmt.Errorf("circuit breaker wrapped health check: %w", err)
	}
	return nil
}

func (p *CircuitBreakerPublisher) Snapshot() PublisherSnapshot {
	if sp, ok := p.wrapped.(SnapshotProvider); ok {
		return sp.Snapshot()
	}
	return PublisherSnapshot{}
}
