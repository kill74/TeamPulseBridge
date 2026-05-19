package dedup

import (
	"sync"
	"time"
)

type Memory struct {
	enabled  bool
	ttl      time.Duration
	mu       sync.Mutex
	seen     map[string]time.Time
	stop     chan struct{}
	stopOnce sync.Once
}

func NewMemory(enabled bool, ttl time.Duration) *Memory {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	m := &Memory{
		enabled: enabled,
		ttl:     ttl,
		seen:    make(map[string]time.Time),
		stop:    make(chan struct{}),
	}
	go m.periodicCleanup()
	return m
}

func (m *Memory) periodicCleanup() {
	ticker := time.NewTicker(m.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanupExpired()
		case <-m.stop:
			return
		}
	}
}

func (m *Memory) cleanupExpired() {
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, ts := range m.seen {
		if !ts.After(now) {
			delete(m.seen, k)
		}
	}
}

func (m *Memory) Stop() {
	m.stopOnce.Do(func() {
		close(m.stop)
	})
}

func (m *Memory) Forget(key string) {
	if !m.enabled || key == "" {
		return
	}
	m.mu.Lock()
	delete(m.seen, key)
	m.mu.Unlock()
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
	return false
}
