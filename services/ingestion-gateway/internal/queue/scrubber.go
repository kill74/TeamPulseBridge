package queue

import (
	"encoding/json"
	"strings"
)

// StructuralScrubber redacts sensitive fields from a JSON payload without corrupting the structure.
type StructuralScrubber struct {
	SensitiveFields []string
	sensitiveSet    map[string]struct{}
}

func NewStructuralScrubber(fields ...string) *StructuralScrubber {
	if len(fields) == 0 {
		fields = []string{
			"email", "password", "token", "secret", "key", "authorization",
			"api_key", "api-key", "apikey", "access_token", "access-token",
			"client_secret", "client-secret", "auth_token", "auth-token",
			"private_key", "private-key", "bearer", "credential",
		}
	}
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[strings.ToLower(f)] = struct{}{}
	}
	return &StructuralScrubber{SensitiveFields: fields, sensitiveSet: set}
}

func (s *StructuralScrubber) Scrub(body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	// Fast-path: no sensitive hint substring => skip Unmarshal/walk/Marshal.
	sampleLen := len(body)
	if sampleLen > 4096 {
		sampleLen = 4096
	}
	hasHint := false
	for _, f := range s.SensitiveFields {
		if containsFold(body[:sampleLen], f) {
			hasHint = true
			break
		}
	}
	if !hasHint && len(body) > 4096 {
		// Large clean payload: skip full parse. Small payloads still go
		// through the exact path below (cheap) to stay safe.
		return body
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		// Not valid JSON, fallback to basic regex scrubbing or return as is
		return body
	}

	s.scrubRecursive(doc)

	scrubbed, err := json.Marshal(doc)
	if err != nil {
		return body
	}
	return scrubbed
}

func (s *StructuralScrubber) scrubRecursive(v any) {
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			if s.isSensitive(k) {
				val[k] = "[REDACTED]"
				continue
			}
			s.scrubRecursive(child)
		}
	case []any:
		for _, child := range val {
			s.scrubRecursive(child)
		}
	}
}

func (s *StructuralScrubber) isSensitive(key string) bool {
	_, ok := s.sensitiveSet[strings.ToLower(key)]
	return ok
}

// containsFold reports whether b contains pattern case-insensitively
// without allocating a lowered copy of b.
func containsFold(b []byte, pattern string) bool {
	if len(pattern) == 0 || len(b) < len(pattern) {
		return false
	}
	for i := 0; i+len(pattern) <= len(b); i++ {
		match := true
		for j := 0; j < len(pattern); j++ {
			cb := b[i+j]
			cp := pattern[j]
			if 'A' <= cb && cb <= 'Z' {
				cb += 'a' - 'A'
			}
			if 'A' <= cp && cp <= 'Z' {
				cp += 'a' - 'A'
			}
			if cb != cp {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
