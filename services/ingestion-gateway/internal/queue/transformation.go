package queue

import (
	"context"
	"fmt"
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
	scrubbed := make([]byte, len(body))
	copy(scrubbed, body)
	for _, transform := range p.transformers {
		scrubbed = transform(scrubbed)
	}
	if err := p.wrapped.Publish(ctx, source, scrubbed, headers); err != nil {
		return fmt.Errorf("transforming publisher publish: %w", err)
	}
	return nil
}

func (p *TransformingPublisher) Close() error {
	if err := p.wrapped.Close(); err != nil {
		return fmt.Errorf("transforming publisher close: %w", err)
	}
	return nil
}

func (p *TransformingPublisher) HealthCheck(ctx context.Context) error {
	if err := p.wrapped.HealthCheck(ctx); err != nil {
		return fmt.Errorf("transforming publisher health check: %w", err)
	}
	return nil
}

func (p *TransformingPublisher) Snapshot() PublisherSnapshot {
	if sp, ok := p.wrapped.(SnapshotProvider); ok {
		return sp.Snapshot()
	}
	return PublisherSnapshot{}
}

func (p *TransformingPublisher) SourceSnapshots() map[string]PublisherSnapshot {
	if sp, ok := p.wrapped.(SourceSnapshotProvider); ok {
		return sp.SourceSnapshots()
	}
	return nil
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
