package retry

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type LeaderElection struct {
	client *redis.Client
	key    string
	ttl    time.Duration
	id     string // unique instance ID
}

func NewLeaderElection(client *redis.Client, key string, id string, ttl time.Duration) *LeaderElection {
	return &LeaderElection{
		client: client,
		key:    key,
		ttl:    ttl,
		id:     id,
	}
}

func (l *LeaderElection) IsLeader(ctx context.Context) bool {
	// SETNX with TTL — only one instance succeeds
	wasSet, err := l.client.SetNX(ctx, l.key, l.id, l.ttl).Result()
	return err == nil && wasSet
}

func (l *LeaderElection) Renew(ctx context.Context) bool {
	// Extend TTL if we're still the leader
	script := `
		if redis.call('GET', KEYS[1]) == ARGV[1] then
			return redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		return 0
	`
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.id, int(l.ttl.Seconds())).Int()
	return err == nil && result == 1
}
