package handlers

import (
	"sync"
	"time"
)

type uiSmokeRateLimiterState struct {
	mu          sync.Mutex
	entries     map[string]*uiRateEntry
	lastCleanup time.Time
}

func newUISmokeRateLimiter() *uiSmokeRateLimiterState {
	return &uiSmokeRateLimiterState{
		entries:     make(map[string]*uiRateEntry),
		lastCleanup: time.Now().UTC(),
	}
}

func (l *uiSmokeRateLimiterState) Stop() {}

func (l *uiSmokeRateLimiterState) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if ip == "" {
		ip = "unknown"
	}

	if now.Sub(l.lastCleanup) >= time.Minute {
		for key, entry := range l.entries {
			if now.Sub(entry.windowStart) >= 2*time.Minute {
				delete(l.entries, key)
			}
		}
		l.lastCleanup = now
	}

	entry := l.entries[ip]
	if entry == nil {
		l.entries[ip] = &uiRateEntry{windowStart: now, count: 1}
		return true
	}

	if now.Sub(entry.windowStart) >= time.Minute {
		entry.windowStart = now
		entry.count = 1
		return true
	}

	if entry.count >= uiSmokeMaxRequestsPerMinute {
		return false
	}

	entry.count++
	return true
}

var uiSmokeRateLimiter = newUISmokeRateLimiter()

type uiRateEntry struct {
	windowStart time.Time
	count       int
}

func allowUISmokeRequest(ip string, now time.Time) bool {
	return uiSmokeRateLimiter.allow(ip, now)
}
