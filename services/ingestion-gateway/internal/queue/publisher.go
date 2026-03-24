package queue

import "context"

// Publisher abstracts publishing raw webhook events to a durable queue.
type Publisher interface {
	Publish(ctx context.Context, source string, body []byte, headers map[string]string) error
}
