package dedup

import (
	"sync"
	"time"
)

type Memory struct {
	enabled bool
	ttl     time.Duration

	mu   sync.Mutex
	seen map[string]time.Time
}

func NewMemory(enabled bool, ttl time.Duration) *Memory {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Memory{
		enabled: enabled,
		ttl:     ttl,
		seen:    make(map[string]time.Time),
	}
}

// Seen returns true if key has already been observed within the dedup window.
func (m *Memory) Seen(key string) bool {
	if !m.enabled || key == "" {
		return false
	}

	now := time.Now().UTC()
	expiry := now.Add(m.ttl)

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.seen[key]; ok && existing.After(now) {
		return true
	}
	m.seen[key] = expiry

	// Opportunistic cleanup to keep memory bounded.
	if len(m.seen) > 10_000 {
		for k, ts := range m.seen {
			if !ts.After(now) {
				delete(m.seen, k)
			}
		}
	}
	return false
}
