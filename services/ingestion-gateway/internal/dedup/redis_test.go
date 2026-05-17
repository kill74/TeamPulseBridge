package dedup

import (
	"errors"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/stretchr/testify/assert"
)

func TestRedis_Forget(t *testing.T) {
	db, mock := redismock.NewClientMock()

	redisDedup := NewRedis(true, db, "prefix", 1*time.Minute)

	t.Run("successfully deletes key", func(t *testing.T) {
		mock.ExpectDel("prefix:test_key").SetVal(1)

		redisDedup.Forget("test_key")

		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("does nothing if disabled", func(t *testing.T) {
		disabledDedup := NewRedis(false, db, "prefix", 1*time.Minute)

		disabledDedup.Forget("test_key")

		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("does nothing if key is empty", func(t *testing.T) {
		redisDedup.Forget("")

		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("does nothing if client is nil", func(t *testing.T) {
		nilClientDedup := NewRedis(true, nil, "prefix", 1*time.Minute)

		nilClientDedup.Forget("test_key")

		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("ignores redis errors", func(t *testing.T) {
		mock.ExpectDel("prefix:error_key").SetErr(errors.New("redis error"))

		redisDedup.Forget("error_key")

		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})
}

func TestRedis_Seen(t *testing.T) {
	db, mock := redismock.NewClientMock()
	ttl := 5 * time.Minute
	redisDedup := NewRedis(true, db, "prefix", ttl)

	t.Run("successfully marks new key as seen", func(t *testing.T) {
		mock.ExpectSetNX("prefix:new_key", "1", ttl).SetVal(true)

		seen := redisDedup.Seen("new_key")

		assert.False(t, seen)
		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("identifies already seen key", func(t *testing.T) {
		mock.ExpectSetNX("prefix:existing_key", "1", ttl).SetVal(false)

		seen := redisDedup.Seen("existing_key")

		assert.True(t, seen)
		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("returns false if disabled", func(t *testing.T) {
		disabledDedup := NewRedis(false, db, "prefix", ttl)

		seen := disabledDedup.Seen("test_key")

		assert.False(t, seen)
		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("returns false if key is empty", func(t *testing.T) {
		seen := redisDedup.Seen("")

		assert.False(t, seen)
		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("returns false if client is nil", func(t *testing.T) {
		nilClientDedup := NewRedis(true, nil, "prefix", ttl)

		seen := nilClientDedup.Seen("test_key")

		assert.False(t, seen)
		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})

	t.Run("fail-open on redis error", func(t *testing.T) {
		mock.ExpectSetNX("prefix:error_key", "1", ttl).SetErr(errors.New("redis error"))

		seen := redisDedup.Seen("error_key")

		assert.False(t, seen) // fail-open means we pretend we haven't seen it so it processes
		err := mock.ExpectationsWereMet()
		assert.NoError(t, err)
	})
}

func TestRedis_Stop(t *testing.T) {
	db, _ := redismock.NewClientMock()
	redisDedup := NewRedis(true, db, "prefix", 1*time.Minute)

	t.Run("successfully closes client", func(t *testing.T) {
		redisDedup.Stop()
	})

	t.Run("handles nil client safely", func(t *testing.T) {
		nilClientDedup := NewRedis(true, nil, "prefix", 1*time.Minute)
		nilClientDedup.Stop() // should not panic
	})
}

func TestNewRedis_DefaultsTTL(t *testing.T) {
	db, _ := redismock.NewClientMock()

	// Test with 0 TTL
	redisDedup := NewRedis(true, db, "prefix", 0)
	assert.Equal(t, 5*time.Minute, redisDedup.ttl)

	// Test with negative TTL
	redisDedup2 := NewRedis(true, db, "prefix", -1*time.Minute)
	assert.Equal(t, 5*time.Minute, redisDedup2.ttl)

	// Test with valid TTL
	customTTL := 10 * time.Minute
	redisDedup3 := NewRedis(true, db, "prefix", customTTL)
	assert.Equal(t, customTTL, redisDedup3.ttl)
}
