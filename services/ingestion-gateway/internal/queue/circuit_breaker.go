package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type CircuitBreaker struct {
	mu            sync.Mutex
	failures      int
	lastFailure   time.Time
	threshold     int
	recoveryDelay time.Duration
	state         string
}

func NewCircuitBreaker(threshold int, recoveryDelay time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:     threshold,
		recoveryDelay: recoveryDelay,
		state:         "closed",
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case "closed":
		return true
	case "open":
		if time.Since(cb.lastFailure) > cb.recoveryDelay {
			cb.state = "half-open"
			return true
		}
		return false
	case "half-open":
		return true
	}
	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = "closed"
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cb.threshold {
		cb.state = "open"
	}
}

func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

type CircuitBreakerPublisher struct {
	wrapped Publisher
	breaker *CircuitBreaker
	logger  *slog.Logger
}

func NewCircuitBreakerPublisher(wrapped Publisher, breaker *CircuitBreaker, logger *slog.Logger) *CircuitBreakerPublisher {
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
