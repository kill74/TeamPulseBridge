package httpx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisRateLimiter struct {
	client  *redis.Client
	prefix  string
	window  time.Duration
	timeout time.Duration
	now     func() time.Time
}

func NewRedisRateLimiter(client *redis.Client, prefix string, window time.Duration) *RedisRateLimiter {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "rate_limit"
	}
	if window < time.Second {
		window = time.Minute
	}
	return &RedisRateLimiter{
		client:  client,
		prefix:  prefix,
		window:  window,
		timeout: 250 * time.Millisecond,
		now:     time.Now,
	}
}

func (l *RedisRateLimiter) Allow(key string, limit int) bool {
	if limit <= 0 {
		return false
	}
	if l == nil || l.client == nil || strings.TrimSpace(key) == "" {
		return true
	}

	windowStart := l.windowStart(l.now().UTC())
	redisKey := l.redisKey(windowStart, key)
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		// Availability-first fail-open behavior. Edge controls and local middleware
		// still protect the service if Redis is temporarily unavailable.
		return true
	}
	if count == 1 {
		_ = l.client.Expire(ctx, redisKey, 2*l.window).Err()
	}
	return count <= int64(limit)
}

func (l *RedisRateLimiter) windowStart(t time.Time) int64 {
	windowSeconds := int64(l.window / time.Second)
	if windowSeconds <= 0 {
		windowSeconds = 60
	}
	unix := t.Unix()
	return unix - (unix % windowSeconds)
}

func (l *RedisRateLimiter) redisKey(windowStart int64, key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%s:%d:%s", l.prefix, windowStart, hex.EncodeToString(sum[:]))
}
