package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/stretchr/testify/assert"
)

func TestLeaderElection_IsLeader(t *testing.T) {
	t.Run("success_becomes_leader", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		defer db.Close()

		key := "leader-key"
		id := "instance-1"
		ttl := time.Minute

		le := NewLeaderElection(db, key, id, ttl)

		ctx := context.Background()
		mock.ExpectSetNX(key, id, ttl).SetVal(true)

		isLeader := le.IsLeader(ctx)
		assert.True(t, isLeader)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("failure_not_leader", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		defer db.Close()

		key := "leader-key"
		id := "instance-1"
		ttl := time.Minute

		le := NewLeaderElection(db, key, id, ttl)

		ctx := context.Background()
		mock.ExpectSetNX(key, id, ttl).SetVal(false)

		isLeader := le.IsLeader(ctx)
		assert.False(t, isLeader)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("redis_error", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		defer db.Close()

		key := "leader-key"
		id := "instance-1"
		ttl := time.Minute

		le := NewLeaderElection(db, key, id, ttl)

		ctx := context.Background()
		mock.ExpectSetNX(key, id, ttl).SetErr(errors.New("redis error"))

		isLeader := le.IsLeader(ctx)
		assert.False(t, isLeader)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestLeaderElection_Renew(t *testing.T) {
	t.Run("success_renews_leadership", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		defer db.Close()

		key := "leader-key"
		id := "instance-1"
		ttl := time.Minute

		le := NewLeaderElection(db, key, id, ttl)

		ctx := context.Background()

		script := `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		return 0
	`
		mock.ExpectEval(script, []string{key}, id, int(ttl.Seconds())).SetVal(int64(1))

		renewed := le.Renew(ctx)
		assert.True(t, renewed)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("failure_not_leader_to_renew", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		defer db.Close()

		key := "leader-key"
		id := "instance-1"
		ttl := time.Minute

		le := NewLeaderElection(db, key, id, ttl)

		ctx := context.Background()

		script := `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		return 0
	`
		mock.ExpectEval(script, []string{key}, id, int(ttl.Seconds())).SetVal(int64(0))

		renewed := le.Renew(ctx)
		assert.False(t, renewed)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("redis_error_on_renew", func(t *testing.T) {
		db, mock := redismock.NewClientMock()
		defer db.Close()

		key := "leader-key"
		id := "instance-1"
		ttl := time.Minute

		le := NewLeaderElection(db, key, id, ttl)

		ctx := context.Background()

		script := `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		return 0
	`
		mock.ExpectEval(script, []string{key}, id, int(ttl.Seconds())).SetErr(errors.New("redis error"))

		renewed := le.Renew(ctx)
		assert.False(t, renewed)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
