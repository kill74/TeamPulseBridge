package replay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Event represents one normalized replay operation.
type Event struct {
	Source  string
	Headers map[string]string
	Body    json.RawMessage
}

// ParseInput parses replay payload bytes.
//
// Supported input formats:
// 1. Raw provider payload JSON (requires sourceOverride).
// 2. Wrapped envelope JSON with top-level source/headers/body fields.
func ParseInput(payload []byte, sourceOverride string, headerOverrides map[string]string) (Event, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return Event{}, errors.New("input payload is empty")
	}

	sourceOverride = strings.TrimSpace(sourceOverride)
	event, wrapped, err := parseWrappedEnvelope(payload)
	if err != nil {
		return Event{}, err
	}
	if !wrapped {
		if sourceOverride == "" {
			return Event{}, errors.New("source is required for raw payload replay (use -source)")
		}
		if !json.Valid(payload) {
			return Event{}, errors.New("raw payload must be valid JSON")
		}
		event = Event{
			Source: sourceOverride,
			Body:   append([]byte(nil), payload...),
		}
	}

	if event.Headers == nil {
		event.Headers = make(map[string]string)
	}
	for k, v := range headerOverrides {
		key := strings.TrimSpace(k)
		if key == "" {
			return Event{}, errors.New("header key must not be empty")
		}
		event.Headers[key] = v
	}

	event.Source = strings.TrimSpace(event.Source)
	if event.Source == "" {
		return Event{}, errors.New("source must not be empty")
	}
	if !json.Valid(event.Body) {
		return Event{}, errors.New("event body must be valid JSON")
	}

	return event, nil
}

type envelopeFile struct {
	Source  string            `json:"source"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func parseWrappedEnvelope(payload []byte) (Event, bool, error) {
	var in envelopeFile
	if err := json.Unmarshal(payload, &in); err != nil {
		if !json.Valid(payload) {
			return Event{}, false, fmt.Errorf("invalid JSON payload: %w", err)
		}
		return Event{}, false, nil
	}
	if strings.TrimSpace(in.Source) == "" || len(bytes.TrimSpace(in.Body)) == 0 {
		return Event{}, false, nil
	}
	return Event{
		Source:  in.Source,
		Headers: in.Headers,
		Body:    append([]byte(nil), in.Body...),
	}, true, nil
}
