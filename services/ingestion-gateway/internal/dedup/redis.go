package dedup

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	enabled atomic.Bool
	client  *redis.Client
	prefix  string
	ttl     time.Duration
}

func NewRedis(enabled bool, client *redis.Client, prefix string, ttl time.Duration) *Redis {
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
func (r *Redis) Seen(key string) bool {
	if !r.enabled.Load() || key == "" || r.client == nil {
		return false
	}

	fullKey := fmt.Sprintf("%s:%s", r.prefix, key)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// SETNX + EXPIRE atomically
	wasSet, err := r.client.SetNX(ctx, fullKey, "1", r.ttl).Result()
	if err != nil {
		// Fallback: allow event if Redis is down (fail-open)
		return false
	}
	return !wasSet // true = already seen (duplicate)
}

func (r *Redis) Forget(key string) {
	if !r.enabled.Load() || key == "" || r.client == nil {
		return
	}
	fullKey := fmt.Sprintf("%s:%s", r.prefix, key)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = r.client.Del(ctx, fullKey).Err()
}

// Stop disables the dedup store without closing the shared Redis client.
// The Redis client lifecycle is managed centrally by the application.
func (r *Redis) Stop() {
	r.enabled.Store(false)
}
