package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type Validator struct {
	mu      sync.RWMutex
	schemas map[string]*jsonSchema
}

type jsonSchema struct {
	Required   []string       `json:"required"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

func NewValidator(schemaPath string) (*Validator, error) {
	v := &Validator{
		schemas: make(map[string]*jsonSchema),
	}

	if schemaPath == "" {
		return v, nil
	}

	if err := v.loadSchemas(schemaPath); err != nil {
		return nil, fmt.Errorf("load schemas: %w", err)
	}

	return v, nil
}

func (v *Validator) loadSchemas(schemaPath string) error {
	files, err := filepath.Glob(filepath.Join(schemaPath, "*.json"))
	if err != nil {
		return fmt.Errorf("glob schema files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no schema files found in %s", schemaPath)
	}

	for _, file := range files {
		source := filepath.Base(file)
		source = source[:len(source)-len(filepath.Ext(source))]

		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read schema file %s: %w", file, err)
		}

		var schema jsonSchema
		if err := json.Unmarshal(data, &schema); err != nil {
			return fmt.Errorf("parse schema file %s: %w", file, err)
		}

		v.schemas[source] = &schema
	}

	return nil
}

func (v *Validator) Validate(source string, body []byte) error {
	v.mu.RLock()
	schema, exists := v.schemas[source]
	v.mu.RUnlock()

	if !exists {
		return nil
	}

	if schema.Type != "object" {
		if !json.Valid(body) {
			return fmt.Errorf("invalid JSON")
		}
		return nil
	}

	fields, err := topLevelObjectFields(body)
	if err != nil {
		return err
	}
	for _, field := range schema.Required {
		if _, ok := fields[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	return nil
}

func topLevelObjectFields(body []byte) (map[string]struct{}, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("invalid JSON object")
	}

	fields := make(map[string]struct{})
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid JSON object key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("invalid JSON object key")
		}
		fields[key] = struct{}{}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("invalid JSON object value: %w", err)
		}
	}

	endTok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid JSON object: %w", err)
	}
	endDelim, ok := endTok.(json.Delim)
	if !ok || endDelim != '}' {
		return nil, fmt.Errorf("invalid JSON object")
	}

	if tok, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("invalid JSON: trailing data after %v", tok)
	} else if err != io.EOF {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return fields, nil
}
