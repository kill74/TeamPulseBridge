package queue

import (
	"encoding/json"
	"strings"
)

// StructuralScrubber redacts sensitive fields from a JSON payload without corrupting the structure.
type StructuralScrubber struct {
	SensitiveFields []string
}

func NewStructuralScrubber(fields ...string) *StructuralScrubber {
	if len(fields) == 0 {
		fields = []string{"email", "password", "token", "secret", "key", "authorization"}
	}
	return &StructuralScrubber{SensitiveFields: fields}
}

func (s *StructuralScrubber) Scrub(body []byte) []byte {
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
	lower := strings.ToLower(key)
	for _, f := range s.SensitiveFields {
		if strings.Contains(lower, f) {
			return true
		}
	}
	return false
}
