package dedup

import "context"

// Store defines the interface for deduplication storage backends.
type Store interface {
	Seen(key string) bool
	Forget(key string)
	Stop()
}

// ContextStore is implemented by stores that can respect request cancellation
// and short timeouts. Handlers should prefer it to avoid 2s stalls.
type ContextStore interface {
	Store
	SeenWithContext(ctx context.Context, key string) bool
	ForgetWithContext(ctx context.Context, key string)
}
