package retry

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/stretchr/testify/assert"
)

func TestLeaderElection_IsLeader(t *testing.T) {
	db, mock := redismock.NewClientMock()

	leader := NewLeaderElection(db, "my-lock-key", "my-node-id", 10*time.Second)

	ctx := context.Background()

	// Test case 1: Successfully becomes leader
	mock.ExpectSetNX("my-lock-key", "my-node-id", 10*time.Second).SetVal(true)
	assert.True(t, leader.IsLeader(ctx))

	// Test case 2: Fails to become leader (someone else has it)
	mock.ExpectSetNX("my-lock-key", "my-node-id", 10*time.Second).SetVal(false)
	assert.False(t, leader.IsLeader(ctx))

	// Test case 3: Error during SETNX
	mock.ExpectSetNX("my-lock-key", "my-node-id", 10*time.Second).SetErr(context.DeadlineExceeded)
	assert.False(t, leader.IsLeader(ctx))

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestLeaderElection_Renew(t *testing.T) {
	db, mock := redismock.NewClientMock()

	leader := NewLeaderElection(db, "my-lock-key", "my-node-id", 10*time.Second)

	ctx := context.Background()

	script := `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		return 0
	`

	// Test case 1: Successfully renewed (is still leader)
	mock.ExpectEval(script, []string{"my-lock-key"}, "my-node-id", 10).SetVal(int64(1))
	assert.True(t, leader.Renew(ctx))

	// Test case 2: Failed to renew (not the leader anymore)
	mock.ExpectEval(script, []string{"my-lock-key"}, "my-node-id", 10).SetVal(int64(0))
	assert.False(t, leader.Renew(ctx))

	// Test case 3: Error during eval
	mock.ExpectEval(script, []string{"my-lock-key"}, "my-node-id", 10).SetErr(context.DeadlineExceeded)
	assert.False(t, leader.Renew(ctx))

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}
