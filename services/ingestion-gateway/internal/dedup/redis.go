package dedup

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Redis struct {
	enabled bool
	client  *redis.Client
	prefix  string
	ttl     time.Duration
	logger  *slog.Logger
}

func NewRedis(enabled bool, client *redis.Client, prefix string, ttl time.Duration, logger *slog.Logger) *Redis {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Redis{
		enabled: enabled,
		client:  client,
		prefix:  prefix,
		ttl:     ttl,
		logger:  logger,
	}
}

// Seen returns true if key has already been observed within the dedup window.
func (r *Redis) Seen(key string) bool {
	if !r.enabled || key == "" || r.client == nil {
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
	if !r.enabled || key == "" || r.client == nil {
		return
	}
	fullKey := fmt.Sprintf("%s:%s", r.prefix, key)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.client.Del(ctx, fullKey).Err(); err != nil {
		if r.logger != nil {
			r.logger.Error("failed to forget dedup key in redis", "key", fullKey, "error", err)
		}
	}
}

// Stop closes the Redis client connections.
func (r *Redis) Stop() {
	if r.client != nil {
		_ = r.client.Close()
	}
}
