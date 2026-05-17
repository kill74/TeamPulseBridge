package config

import (
	"sync"
	"sync/atomic"
)

type FeatureFlags struct {
	mu    sync.RWMutex
	flags map[string]*atomic.Bool
}

func NewFeatureFlags() *FeatureFlags {
	return &FeatureFlags{
		flags: make(map[string]*atomic.Bool),
	}
}

func (f *FeatureFlags) Register(name string, enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	val := &atomic.Bool{}
	val.Store(enabled)
	f.flags[name] = val
}

func (f *FeatureFlags) IsEnabled(name string) bool {
	f.mu.RLock()
	flag, ok := f.flags[name]
	f.mu.RUnlock()
	if !ok {
		return false
	}
	return flag.Load()
}

func (f *FeatureFlags) Set(name string, enabled bool) bool {
	f.mu.RLock()
	flag, ok := f.flags[name]
	f.mu.RUnlock()
	if !ok {
		return false
	}
	flag.Store(enabled)
	return true
}

func (f *FeatureFlags) List() map[string]bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make(map[string]bool, len(f.flags))
	for name, flag := range f.flags {
		result[name] = flag.Load()
	}
	return result
}

func (f *FeatureFlags) All() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	names := make([]string, 0, len(f.flags))
	for name := range f.flags {
		names = append(names, name)
	}
	return names
}
