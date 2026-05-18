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
	result := l.AllowWithInfo(key, limit, l.now())
	return result.Allowed
}

func (l *RedisRateLimiter) AllowWithInfo(key string, limit int, now time.Time) RateLimitResult {
	if limit <= 0 {
		return RateLimitResult{Allowed: false, Limit: limit}
	}
	if l.client == nil || strings.TrimSpace(key) == "" {
		return RateLimitResult{Allowed: true, Limit: limit, Remaining: limit}
	}

	windowStart := l.windowStart(now.UTC())
	redisKey := l.redisKey(windowStart, key)
	resetAt := time.Unix(windowStart+int64(l.window/time.Second), 0).UTC()
	ctx, cancel := context.WithTimeout(context.Background(), l.timeout)
	defer cancel()

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return RateLimitResult{
			Allowed:   true,
			Remaining: limit,
			ResetAt:   resetAt,
			Limit:     limit,
		}
	}
	if count == 1 {
		_ = l.client.Expire(ctx, redisKey, 2*l.window).Err()
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}
	return RateLimitResult{
		Allowed:   count <= int64(limit),
		Remaining: remaining,
		ResetAt:   resetAt,
		Limit:     limit,
	}
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
