package dedup

import (
	"testing"
	"time"
)

func TestRedisSeen_DisabledOrInvalid(t *testing.T) {
	// Test when Redis deduplication is disabled
	r1 := NewRedis(false, nil, "prefix", time.Minute)
	if r1.Seen("key") {
		t.Fatal("disabled dedup should return false")
	}
	if r1.Seen("key") {
		t.Fatal("disabled dedup should return false repeatedly")
	}

	// Test when Redis is enabled but client is nil
	r2 := NewRedis(true, nil, "prefix", time.Minute)
	if r2.Seen("key") {
		t.Fatal("nil client should return false")
	}

	// Test when key is empty
	r3 := NewRedis(true, nil, "prefix", time.Minute)
	if r3.Seen("") {
		t.Fatal("empty key should return false")
	}
}

func TestRedisForget_DisabledOrInvalid(t *testing.T) {
	// Should not panic or do anything when disabled
	r1 := NewRedis(false, nil, "prefix", time.Minute)
	r1.Forget("key")

	// Should not panic when client is nil
	r2 := NewRedis(true, nil, "prefix", time.Minute)
	r2.Forget("key")

	// Should not panic when key is empty
	r3 := NewRedis(true, nil, "prefix", time.Minute)
	r3.Forget("")
}

func TestRedisStop_NilClient(t *testing.T) {
	// Should not panic when stopping with a nil client
	r := NewRedis(true, nil, "prefix", time.Minute)
	r.Stop()
}
