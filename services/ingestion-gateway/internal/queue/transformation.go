package queue

import (
	"context"
	"regexp"
)

// Transformer defines a function that can modify the payload before it is published.
type Transformer func(body []byte) []byte

// TransformingPublisher wraps an existing publisher and applies a set of transformations to every payload.
type TransformingPublisher struct {
	wrapped      Publisher
	transformers []Transformer
}

func NewTransformingPublisher(wrapped Publisher, transformers ...Transformer) *TransformingPublisher {
	return &TransformingPublisher{
		wrapped:      wrapped,
		transformers: transformers,
	}
}

func (p *TransformingPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	scrubbed := body
	for _, transform := range p.transformers {
		scrubbed = transform(scrubbed)
	}
	return p.wrapped.Publish(ctx, source, scrubbed, headers)
}

func (p *TransformingPublisher) Close() error {
	return p.wrapped.Close()
}

func (p *TransformingPublisher) HealthCheck(ctx context.Context) error {
	return p.wrapped.HealthCheck(ctx)
}

// PII Scrubbing Transformers

var (
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	tokenRegex = regexp.MustCompile(`(?i)(["']?(?:bearer|token|secret|password|key)["']?)(\s*[=:]\s*|\s+)(["']?)[a-zA-Z0-9\-_.~%]+(["']?)`)
)

func ScrubEmails(body []byte) []byte {
	return emailRegex.ReplaceAll(body, []byte("[REDACTED_EMAIL]"))
}

func ScrubTokens(body []byte) []byte {
	return tokenRegex.ReplaceAll(body, []byte("${1}${2}${3}[REDACTED_SECRET]${4}"))
}
