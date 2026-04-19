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

func TestMemorySeen_Disabled(t *testing.T) {
	d := NewMemory(false, time.Minute)
	if d.Seen("github:abc") {
		t.Fatal("disabled dedup should never mark duplicates")
	}
	if d.Seen("github:abc") {
		t.Fatal("disabled dedup should never mark duplicates")
	}
}
