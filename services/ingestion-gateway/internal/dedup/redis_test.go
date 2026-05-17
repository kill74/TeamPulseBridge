package dedup

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// mockRedisClient creates a go-redis client.
// Since testing Close() directly without a real redis connection or a mocked interface
// in go-redis can be tricky, we test that the underlying connection pool gets closed.
func TestRedisStop(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		r := NewRedis(true, nil, "prefix", time.Minute)
		// Should not panic
		assert.NotPanics(t, func() {
			r.Stop()
		})
	})

	t.Run("with client", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{})
		r := NewRedis(true, client, "prefix", time.Minute)

		assert.NotPanics(t, func() {
			r.Stop()
		})

		// Attempting to do a command after stop should yield an error indicating connection closed
		err := client.Ping(context.Background()).Err()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "redis: client is closed")
	})
}
