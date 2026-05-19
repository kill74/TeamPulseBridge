package queue

import (
	"context"
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
		return err
	}
	p.breaker.RecordSuccess()
	return nil
}

func (p *CircuitBreakerPublisher) Close() error {
	p.breaker.Close()
	return p.wrapped.Close()
}

func (p *CircuitBreakerPublisher) HealthCheck(ctx context.Context) error {
	return p.wrapped.HealthCheck(ctx)
}

func (p *CircuitBreakerPublisher) Snapshot() PublisherSnapshot {
	if sp, ok := p.wrapped.(SnapshotProvider); ok {
		return sp.Snapshot()
	}
	return PublisherSnapshot{}
}
