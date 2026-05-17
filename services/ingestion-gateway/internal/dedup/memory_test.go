package dedup

import (
	"testing"
	"time"
)

func TestMemorySeen_DeduplicatesWithinTTL(t *testing.T) {
	d := NewMemory(true, time.Minute)
	if d.Seen("github:abc") {
		t.Fatal("first event should not be marked duplicate")
	}
	if !d.Seen("github:abc") {
		t.Fatal("second event should be marked duplicate")
	}
}

func TestMemoryForget_AllowsRetryAfterFailedPublish(t *testing.T) {
	d := NewMemory(true, time.Minute)
	if d.Seen("github:abc") {
		t.Fatal("first event should not be marked duplicate")
	}
	d.Forget("github:abc")
	if d.Seen("github:abc") {
		t.Fatal("forgotten event should be accepted again")
	}
}

func TestMemorySeen_Disabled(t *testing.T) {
	d := NewMemory(false, time.Minute)
	if d.Seen("github:abc") {
		t.Fatal("disabled dedup should never mark duplicates")
	}
	if d.Seen("github:abc") {
		t.Fatal("disabled dedup should never mark duplicates")
	}
}

func TestMemorySeen_CleansUpExpiredEntries(t *testing.T) {
	ttl := 50 * time.Millisecond
	d := NewMemory(true, ttl)

	if d.Seen("test:1") {
		t.Fatal("first event should not be marked duplicate")
	}

	time.Sleep(ttl * 2)

	if d.Seen("test:1") {
		t.Fatal("expired entry should not be marked duplicate after TTL")
	}

	d.Stop()
}

func TestMemoryStop_DoesNotPanic(t *testing.T) {
	d := NewMemory(true, time.Minute)
	d.Stop()

	if d.Seen("test:1") {
		t.Fatal("should not panic after stop")
	}
}
