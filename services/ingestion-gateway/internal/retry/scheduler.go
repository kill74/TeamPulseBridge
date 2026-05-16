package retry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

type Scheduler struct {
	store      failstore.Store
	publisher  queue.Publisher
	logger     *slog.Logger
	maxRetries int
	interval   time.Duration
	ticker     *time.Ticker
	stop       chan struct{}
	stopOnce   sync.Once
	mu         sync.Mutex
	running    bool
	onRetry    func(ctx context.Context, source string, success bool, attempt int)
	rng        *rand.Rand
	retries    sync.Map
}

type SchedulerOptions struct {
	MaxRetries int
	Interval   time.Duration
	OnRetry    func(ctx context.Context, source string, success bool, attempt int)
}

func NewScheduler(store failstore.Store, publisher queue.Publisher, logger *slog.Logger, opts SchedulerOptions) *Scheduler {
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	if opts.Interval <= 0 {
		opts.Interval = 10 * time.Second
	}

	return &Scheduler{
		store:      store,
		publisher:  publisher,
		logger:     logger,
		maxRetries: opts.MaxRetries,
		interval:   opts.Interval,
		stop:       make(chan struct{}),
		onRetry:    opts.OnRetry,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}

	s.running = true
	s.ticker = time.NewTicker(s.interval)

	go s.run()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.stopOnce.Do(func() {
		if s.ticker != nil {
			s.ticker.Stop()
		}
		close(s.stop)
	})
}

func (s *Scheduler) run() {
	for {
		select {
		case <-s.ticker.C:
			s.processRetries()
		case <-s.stop:
			return
		}
	}
}

func (s *Scheduler) processRetries() {
	ctx := context.Background()

	events, err := s.store.ListRecent(ctx, 50)
	if err != nil {
		s.logger.Error("failed to list events for retry", "error", err)
		return
	}

	for _, event := range events {
		currentRetries := 0
		if val, ok := s.retries.Load(event.EventID); ok {
			currentRetries = val.(int)
		}

		if currentRetries >= s.maxRetries {
			continue
		}

		backoff := s.calculateBackoff(currentRetries, s.interval)
		if time.Since(event.FailedAt) < backoff {
			continue
		}

		s.retryEvent(ctx, event)
	}
}

func (s *Scheduler) retryEvent(ctx context.Context, event failstore.FailedEvent) {
	currentRetries := 0
	if val, ok := s.retries.Load(event.EventID); ok {
		currentRetries = val.(int)
	}
	nextRetries := currentRetries + 1

	if nextRetries > s.maxRetries {
		s.logger.Info("max retries exceeded, skipping",
			"event_id", event.EventID,
			"max_retries", s.maxRetries,
		)
		return
	}

	s.retries.Store(event.EventID, nextRetries)

	headers := make(map[string]string, len(event.Headers)+1)
	for k, v := range event.Headers {
		headers[k] = v
	}
	headers["X-Retry-Count"] = fmt.Sprintf("%d", nextRetries)

	err := s.publisher.Publish(ctx, event.Source, event.Body, headers)

	success := err == nil
	if s.onRetry != nil {
		s.onRetry(ctx, event.Source, success, nextRetries)
	}

	if success {
		s.logger.Info("event retried successfully",
			"event_id", event.EventID,
			"source", event.Source,
			"attempt", nextRetries,
		)
		s.retries.Delete(event.EventID)
	} else {
		s.logger.Warn("event retry failed",
			"event_id", event.EventID,
			"source", event.Source,
			"attempt", nextRetries,
			"error", err,
		)
	}
}

func (s *Scheduler) calculateBackoff(retryCount int, baseInterval time.Duration) time.Duration {
	if retryCount < 0 {
		retryCount = 0
	}

	exp := math.Pow(2, float64(retryCount))
	backoff := time.Duration(float64(baseInterval) * exp)

	s.mu.Lock()
	jitter := time.Duration(s.rng.Float64() * float64(baseInterval))
	s.mu.Unlock()
	backoff += jitter

	maxBackoff := 5 * time.Minute
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	return backoff
}
