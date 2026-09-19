package dedup

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	enabled atomic.Bool
	client  redis.UniversalClient
	prefix  string
	ttl     time.Duration
}

func NewRedis(enabled bool, client redis.UniversalClient, prefix string, ttl time.Duration) *Redis {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	r := &Redis{
		client: client,
		prefix: prefix,
		ttl:    ttl,
	}
	r.enabled.Store(enabled)
	return r
}

// Seen returns true if key has already been observed within the dedup window.
// Uses a short 100ms budget and fail-open semantics so a Redis brownout
// cannot stall the webhook hot path.
func (r *Redis) Seen(key string) bool {
	return r.SeenWithContext(context.Background(), key)
}

// SeenWithContext respects request cancellation and caps latency at 100ms.
func (r *Redis) SeenWithContext(ctx context.Context, key string) bool {
	if !r.enabled.Load() || key == "" || r.client == nil {
		return false
	}

	fullKey := r.prefix + ":" + key
	timeoutCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	// SETNX + EXPIRE atomically
	wasSet, err := r.client.SetNX(timeoutCtx, fullKey, "1", r.ttl).Result()
	if err != nil {
		// Fallback: allow event if Redis is down (fail-open)
		return false
	}
	return !wasSet // true = already seen (duplicate)
}

func (r *Redis) Forget(key string) {
	r.ForgetWithContext(context.Background(), key)
}

// ForgetWithContext only forgets on retriable paths; callers should avoid
// calling it on terminal validation failures.
func (r *Redis) ForgetWithContext(ctx context.Context, key string) {
	if !r.enabled.Load() || key == "" || r.client == nil {
		return
	}
	fullKey := r.prefix + ":" + key
	timeoutCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_ = r.client.Del(timeoutCtx, fullKey).Err()
}

// Stop disables the dedup store without closing the shared Redis client.
// The Redis client lifecycle is managed centrally by the application.
func (r *Redis) Stop() {
	r.enabled.Store(false)
}
