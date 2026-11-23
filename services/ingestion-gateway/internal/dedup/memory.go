package dedup

import (
	"hash/fnv"
	"sync"
	"time"
)

const memoryShards = 32

// memoryShard is one stripe of the sharded dedup map. Striping spreads the
// global lock that previously blocked all webhooks on every Seen().
type memoryShard struct {
	mu   sync.RWMutex
	seen map[string]time.Time
}

type Memory struct {
	enabled  bool
	ttl      time.Duration
	shards   [memoryShards]*memoryShard
	stop     chan struct{}
	stopOnce sync.Once
	// maxEntries caps memory under key-flood; 0 = unbounded (back-compat).
	maxEntries int
}

func NewMemory(enabled bool, ttl time.Duration) *Memory {
	return NewMemoryWithCap(enabled, ttl, 200000)
}

func NewMemoryWithCap(enabled bool, ttl time.Duration, maxEntries int) *Memory {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	m := &Memory{
		enabled:    enabled,
		ttl:        ttl,
		stop:       make(chan struct{}),
		maxEntries: maxEntries,
	}
	for i := range m.shards {
		m.shards[i] = &memoryShard{seen: make(map[string]time.Time)}
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

func (m *Memory) shardFor(key string) *memoryShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return m.shards[h.Sum32()%memoryShards]
}

func (m *Memory) cleanupExpired() {
	now := time.Now().UTC()
	// Incremental: clean one shard per tick rotation would be ideal; full sweep
	// here is still cheap because each shard lock is held independently.
	for _, sh := range m.shards {
		sh.mu.Lock()
		for k, ts := range sh.seen {
			if !ts.After(now) {
				delete(sh.seen, k)
			}
		}
		// Opportunistic cap: drop oldest-expired overflow without growing unbounded.
		if m.maxEntries > 0 {
			perShard := m.maxEntries / memoryShards
			if len(sh.seen) > perShard*2 {
				n := 0
				for k := range sh.seen {
					delete(sh.seen, k)
					n++
					if n > perShard {
						break
					}
				}
			}
		}
		sh.mu.Unlock()
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
	sh := m.shardFor(key)
	sh.mu.Lock()
	delete(sh.seen, key)
	sh.mu.Unlock()
}

// Seen returns true if key has already been observed within the dedup window.
func (m *Memory) Seen(key string) bool {
	return m.SeenWithContext(key)
}

// SeenWithContext is the fast path: RWMutex read first, write only on insert.
func (m *Memory) SeenWithContext(key string) bool {
	if !m.enabled || key == "" {
		return false
	}
	now := time.Now().UTC()
	sh := m.shardFor(key)
	sh.mu.RLock()
	expiry, ok := sh.seen[key]
	if ok && expiry.After(now) {
		sh.mu.RUnlock()
		return true
	}
	sh.mu.RUnlock()

	sh.mu.Lock()
	// Re-check under write lock (lost race).
	if expiry, ok := sh.seen[key]; ok && expiry.After(time.Now().UTC()) {
		sh.mu.Unlock()
		return true
	}
	sh.seen[key] = now.Add(m.ttl)
	sh.mu.Unlock()
	return false
}
