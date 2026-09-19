package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type CircuitBreaker struct {
	mu            sync.RWMutex
	failures      int
	successes     int
	lastFailure   time.Time
	threshold     int
	recoveryDelay time.Duration
	state         string
	// halfOpenInFlight gates the thundering herd: only one probe at a time.
	halfOpenInFlight atomic.Bool
}

func NewCircuitBreaker(threshold int, recoveryDelay time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 25
	}
	if recoveryDelay <= 0 {
		recoveryDelay = 8 * time.Second
	}
	return &CircuitBreaker{
		threshold:     threshold,
		recoveryDelay: recoveryDelay,
		state:         "closed",
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	state := cb.state
	lastFailure := cb.lastFailure
	recoveryDelay := cb.recoveryDelay
	cb.mu.RUnlock()
	switch state {
	case "closed":
		return true
	case "open":
		if time.Since(lastFailure) > recoveryDelay {
			// Single-flight probe: first goroutine becomes the probe,
			// others fast-fail until the probe resolves.
			if cb.halfOpenInFlight.CompareAndSwap(false, true) {
				cb.mu.Lock()
				// Re-check under lock; another probe may have transitioned.
				if cb.state == "open" && time.Since(cb.lastFailure) > cb.recoveryDelay {
					cb.state = "half-open"
				}
				cb.mu.Unlock()
				return true
			}
			return false
		}
		return false
	case "half-open":
		// Probe already in flight — fast-fail others until it resolves.
		return false
	}
	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.successes++
	cb.state = "closed"
	cb.halfOpenInFlight.Store(false)
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.successes = 0
	cb.lastFailure = time.Now()
	if cb.failures >= cb.threshold {
		cb.state = "open"
	}
	cb.halfOpenInFlight.Store(false)
}

func (cb *CircuitBreaker) State() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
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
