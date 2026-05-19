package resilience

import (
	"context"
	"errors"
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

func (cb *CircuitBreaker) Close() error {
	return nil
}

func Execute[T any](ctx context.Context, cb *CircuitBreaker, fn func(ctx context.Context) (T, error)) (T, error) {
	if !cb.Allow() {
		var zero T
		return zero, ErrCircuitOpen
	}

	res, err := fn(ctx)
	if err != nil {
		cb.RecordFailure()
		return res, err
	}

	cb.RecordSuccess()
	return res, nil
}
