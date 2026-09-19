package httpx

import (
	"context"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisRateLimiter struct {
	client  redis.UniversalClient
	prefix  string
	window  time.Duration
	timeout time.Duration
	now     func() time.Time
}

func NewRedisRateLimiter(client redis.UniversalClient, prefix string, window time.Duration) *RedisRateLimiter {
	return NewRedisRateLimiterWithTimeout(client, prefix, window, 50*time.Millisecond)
}

// NewRedisRateLimiterWithTimeout lets services use a tight 20-50ms budget in
// the hot path so Redis saturation fails open fast instead of stalling.
func NewRedisRateLimiterWithTimeout(client redis.UniversalClient, prefix string, window time.Duration, timeout time.Duration) *RedisRateLimiter {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "rate_limit"
	}
	if window < time.Second {
		window = time.Minute
	}
	if timeout <= 0 || timeout > 500*time.Millisecond {
		timeout = 50 * time.Millisecond
	}
	return &RedisRateLimiter{
		client:  client,
		prefix:  prefix,
		window:  window,
		timeout: timeout,
		now:     time.Now,
	}
}

func (l *RedisRateLimiter) Allow(key string, limit int) bool {
	result := l.AllowWithInfo(key, limit, l.now())
	return result.Allowed
}

var incrWithExpireScript = redis.NewScript(`
	local count = redis.call('INCR', KEYS[1])
	if count == 1 then
		redis.call('EXPIRE', KEYS[1], ARGV[1])
	end
	return count
`)

// combinedCheckScript collapses 3 RTTs (general INCR + source INCR + dedup SETNX)
// into a single EVALSHA. Returns {general_count, source_count, dedup_was_set}.
var combinedCheckScript = redis.NewScript(`
	local general = redis.call('INCR', KEYS[1])
	if general == 1 then
		redis.call('EXPIRE', KEYS[1], ARGV[1])
	end
	local source = redis.call('INCR', KEYS[2])
	if source == 1 then
		redis.call('EXPIRE', KEYS[2], ARGV[1])
	end
	local dedup_set = 0
	if KEYS[3] ~= '' then
		local res = redis.call('SET', KEYS[3], '1', 'EX', ARGV[2], 'NX')
		if res then
			dedup_set = 1
		end
	end
	return {general, source, dedup_set}
`)

// CombinedResult is the single-RTT outcome for rate-limit + dedup.
type CombinedResult struct {
	GeneralCount   int
	SourceCount    int
	GeneralAllowed bool
	SourceAllowed  bool
	IsDuplicate    bool
	ResetAt        time.Time
	GeneralLimit   int
	SourceLimit    int
}

// CombinedCheckWithContext runs general + source increments and dedup SETNX in
// one Lua invocation (fail-open on any Redis error). Pass dedupKey="" to skip
// dedup, or dedupTTL<=0 to use the limiter window for dedup expiry.
func (l *RedisRateLimiter) CombinedCheckWithContext(ctx context.Context, generalKey, sourceKey, dedupKey string, generalLimit, sourceLimit int, dedupTTL time.Duration) CombinedResult {
	now := l.now().UTC()
	windowStart := l.windowStart(now)
	resetAt := time.Unix(windowStart+int64(l.window/time.Second), 0).UTC()
	windowSec := int64(l.window / time.Second)
	if windowSec <= 0 {
		windowSec = 60
	}
	dedupSec := int64(dedupTTL / time.Second)
	if dedupSec <= 0 {
		dedupSec = windowSec
	}
	failOpen := func() CombinedResult {
		return CombinedResult{
			GeneralCount: 0, SourceCount: 0,
			GeneralAllowed: true, SourceAllowed: true,
			IsDuplicate: false, ResetAt: resetAt,
			GeneralLimit: generalLimit, SourceLimit: sourceLimit,
		}
	}
	if l.client == nil {
		return failOpen()
	}
	gKey := ""
	if strings.TrimSpace(generalKey) != "" {
		gKey = l.redisKey(windowStart, generalKey)
	} else {
		// Dummy key that still increments harmlessly? Use empty-slot pattern:
		// instead send a throwaway key in our namespace to keep script arity.
		gKey = l.prefix + ":noop:" + strconv.FormatInt(windowStart, 10)
	}
	sKey := ""
	if strings.TrimSpace(sourceKey) != "" {
		sKey = l.redisKey(windowStart, sourceKey)
	} else {
		sKey = l.prefix + ":noop:" + strconv.FormatInt(windowStart, 10)
	}
	dKey := strings.TrimSpace(dedupKey)
	timeoutCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	vals, err := combinedCheckScript.Run(timeoutCtx, l.client, []string{gKey, sKey, dKey}, windowSec, dedupSec).Slice()
	if err != nil || len(vals) != 3 {
		return failOpen()
	}
	toInt := func(v any) int {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		default:
			return 0
		}
	}
	gCount := toInt(vals[0])
	sCount := toInt(vals[1])
	wasSet := toInt(vals[2]) == 1
	return CombinedResult{
		GeneralCount:   gCount,
		SourceCount:    sCount,
		GeneralAllowed: generalLimit <= 0 || gCount <= generalLimit,
		SourceAllowed:  sourceLimit <= 0 || sCount <= sourceLimit,
		IsDuplicate:    dKey != "" && !wasSet,
		ResetAt:        resetAt,
		GeneralLimit:   generalLimit,
		SourceLimit:    sourceLimit,
	}
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

	count, err := incrWithExpireScript.Run(ctx, l.client, []string{redisKey}, int64(l.window/time.Second)).Int()
	if err != nil {
		return RateLimitResult{
			Allowed:   true,
			Remaining: limit,
			ResetAt:   resetAt,
			Limit:     limit,
		}
	}

	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return RateLimitResult{
		Allowed:   count <= limit,
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
	// FNV-1a is ~10x cheaper than SHA256 for short rate-limit keys and does
	// not need cryptographic strength here (just shard + collision avoidance).
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	// Decimal avoids hex.EncodeToString alloc.
	return l.prefix + ":" + strconv.FormatInt(windowStart, 10) + ":" + strconv.FormatUint(h.Sum64(), 16)
}

// AllowWithContext respects request cancellation; use it in middleware.
func (l *RedisRateLimiter) AllowWithContext(ctx context.Context, key string, limit int) bool {
	result := l.allowWithContext(ctx, key, limit, l.now())
	return result.Allowed
}

func (l *RedisRateLimiter) allowWithContext(ctx context.Context, key string, limit int, now time.Time) RateLimitResult {
	if limit <= 0 {
		return RateLimitResult{Allowed: false, Limit: limit}
	}
	if l.client == nil || strings.TrimSpace(key) == "" {
		return RateLimitResult{Allowed: true, Limit: limit, Remaining: limit}
	}
	windowStart := l.windowStart(now.UTC())
	redisKey := l.redisKey(windowStart, key)
	resetAt := time.Unix(windowStart+int64(l.window/time.Second), 0).UTC()
	timeoutCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	count, err := incrWithExpireScript.Run(timeoutCtx, l.client, []string{redisKey}, int64(l.window/time.Second)).Int()
	if err != nil {
		return RateLimitResult{
			Allowed:   true,
			Remaining: limit,
			ResetAt:   resetAt,
			Limit:     limit,
		}
	}
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return RateLimitResult{
		Allowed:   count <= limit,
		Remaining: remaining,
		ResetAt:   resetAt,
		Limit:     limit,
	}
}
