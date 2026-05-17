package dedup

// Store defines the interface for deduplication storage backends.
type Store interface {
	Seen(key string) bool
	Forget(key string)
	Stop()
}
