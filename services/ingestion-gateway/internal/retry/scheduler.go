package retry

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
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
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	running    bool
	stopped    bool
	onRetry    func(ctx context.Context, source string, success bool, attempt int)
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
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running || s.stopped {
		return
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	s.ticker = time.NewTicker(s.interval)
	s.wg.Add(1)
	go s.run(runCtx)
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		s.wg.Wait()
		return
	}
	s.running = false
	s.stopped = true
	if s.ticker != nil {
		s.ticker.Stop()
	}
	if s.cancel != nil {
		s.cancel()
	}
	close(s.stop)
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-s.ticker.C:
			s.processRetries(ctx)
		case <-s.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) processRetries(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()

	events, err := s.store.ListRecent(ctx, 50)
	if err != nil {
		s.logger.Error("failed to list events for retry", "error", err)
		return
	}

	seenEventIDs := make(map[string]struct{}, len(events))
	for _, event := range events {
		seenEventIDs[event.EventID] = struct{}{}
		select {
		case <-ctx.Done():
			return
		default:
		}
		currentRetries := 0
		if val, ok := s.retries.Load(event.EventID); ok {
			if n, ok := val.(int); ok {
				currentRetries = n
			}
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
	s.pruneRetryState(seenEventIDs)
}

func (s *Scheduler) pruneRetryState(active map[string]struct{}) {
	s.retries.Range(func(key, _ any) bool {
		eventID, ok := key.(string)
		if !ok {
			s.retries.Delete(key)
			return true
		}
		if _, exists := active[eventID]; !exists {
			s.retries.Delete(key)
		}
		return true
	})
}

func (s *Scheduler) retryEvent(ctx context.Context, event failstore.FailedEvent) {
	currentRetries := 0
	if val, ok := s.retries.Load(event.EventID); ok {
		if n, ok := val.(int); ok {
			currentRetries = n
		}
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

	jitterBytes := make([]byte, 8)
	if _, err := rand.Read(jitterBytes); err == nil {
		var jitterVal uint64
		for i := 0; i < 8; i++ {
			jitterVal = (jitterVal << 8) | uint64(jitterBytes[i])
		}
		jitter := time.Duration(float64(jitterVal) / float64(^uint64(0)) * float64(baseInterval))
		backoff += jitter
	}

	maxBackoff := 5 * time.Minute
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	return backoff
}
