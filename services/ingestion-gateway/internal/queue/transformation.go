package queue

import (
	"bytes"
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
	// Fast-path: skip regex entirely when there's nothing to redact.
	if len(body) == 0 {
		if err := p.wrapped.Publish(ctx, source, body, headers); err != nil {
			return fmt.Errorf("transforming publisher publish: %w", err)
		}
		return nil
	}
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

// SizeGatedScrubPublisher skips expensive scrubbing above maxBytes (large
// payloads are passed through; operators should scrub those async offline).
// Below the gate it behaves like TransformingPublisher.
type SizeGatedScrubPublisher struct {
	wrapped      Publisher
	transformers []Transformer
	maxBytes     int
}

func NewSizeGatedScrubPublisher(wrapped Publisher, maxBytes int, transformers ...Transformer) *SizeGatedScrubPublisher {
	return &SizeGatedScrubPublisher{wrapped: wrapped, transformers: transformers, maxBytes: maxBytes}
}

func (p *SizeGatedScrubPublisher) Publish(ctx context.Context, source string, body []byte, headers map[string]string) error {
	if p.maxBytes > 0 && len(body) > p.maxBytes {
		// Large payload fast-path: enqueue as-is to keep workers flowing.
		if err := p.wrapped.Publish(ctx, source, body, headers); err != nil {
			return fmt.Errorf("size-gated scrub publish (passthrough): %w", err)
		}
		return nil
	}
	scrubbed := body
	// Single copy, then in-place transforms.
	if len(p.transformers) > 0 {
		buf := make([]byte, len(body))
		copy(buf, body)
		scrubbed = buf
		for _, transform := range p.transformers {
			scrubbed = transform(scrubbed)
		}
	}
	if err := p.wrapped.Publish(ctx, source, scrubbed, headers); err != nil {
		return fmt.Errorf("size-gated scrub publish: %w", err)
	}
	return nil
}

func (p *SizeGatedScrubPublisher) Close() error {
	if err := p.wrapped.Close(); err != nil {
		return fmt.Errorf("size-gated scrub close: %w", err)
	}
	return nil
}

func (p *SizeGatedScrubPublisher) HealthCheck(ctx context.Context) error {
	if err := p.wrapped.HealthCheck(ctx); err != nil {
		return fmt.Errorf("size-gated scrub health check: %w", err)
	}
	return nil
}

func (p *SizeGatedScrubPublisher) Snapshot() PublisherSnapshot {
	if sp, ok := p.wrapped.(SnapshotProvider); ok {
		return sp.Snapshot()
	}
	return PublisherSnapshot{}
}

func (p *SizeGatedScrubPublisher) SourceSnapshots() map[string]PublisherSnapshot {
	if sp, ok := p.wrapped.(SourceSnapshotProvider); ok {
		return sp.SourceSnapshots()
	}
	return nil
}

var (
	emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	tokenRegex = regexp.MustCompile(`(?i)(["']?(?:bearer|token|secret|password|key)["']?)([=:]\s*|\s+)(["']?)[a-zA-Z0-9\-_.~%]+(["']?)`)
)

func ScrubEmails(body []byte) []byte {
	// Pre-filter: no '@' means no email, skip regex over up-to-1MiB bodies.
	if len(body) == 0 || bytes.IndexByte(body, '@') == -1 {
		return body
	}
	return emailRegex.ReplaceAll(body, []byte("[REDACTED_EMAIL]"))
}

func ScrubTokens(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	// Cheap pre-filter: secrets almost always contain '=' or ':' separators.
	// Skips the expensive case-insensitive regex on clean payloads.
	if bytes.IndexByte(body, '=') == -1 && bytes.IndexByte(body, ':') == -1 {
		return body
	}
	return tokenRegex.ReplaceAll(body, []byte("${1}${2}${3}[REDACTED_SECRET]${4}"))
}
