package handlers

import (
	"sync"
	"time"
)

var uiSmokeRateLimiter = struct {
	mu          sync.Mutex
	entries     map[string]*uiRateEntry
	lastCleanup time.Time
}{
	entries:     map[string]*uiRateEntry{},
	lastCleanup: time.Now(),
}

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			uiSmokeRateLimiter.mu.Lock()
			cutoff := time.Now().Add(-time.Minute)
			for ip, entry := range uiSmokeRateLimiter.entries {
				if entry.windowStart.Before(cutoff) {
					delete(uiSmokeRateLimiter.entries, ip)
				}
			}
			uiSmokeRateLimiter.lastCleanup = time.Now()
			uiSmokeRateLimiter.mu.Unlock()
		}
	}()
}

type uiRateEntry struct {
	windowStart time.Time
	count       int
}

func allowUISmokeRequest(ip string, now time.Time) bool {
	uiSmokeRateLimiter.mu.Lock()
	defer uiSmokeRateLimiter.mu.Unlock()

	if ip == "" {
		ip = "unknown"
	}

	if now.Sub(uiSmokeRateLimiter.lastCleanup) >= time.Minute {
		for key, entry := range uiSmokeRateLimiter.entries {
			if now.Sub(entry.windowStart) >= 2*time.Minute {
				delete(uiSmokeRateLimiter.entries, key)
			}
		}
		uiSmokeRateLimiter.lastCleanup = now
	}

	entry := uiSmokeRateLimiter.entries[ip]
	if entry == nil {
		uiSmokeRateLimiter.entries[ip] = &uiRateEntry{windowStart: now, count: 1}
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
