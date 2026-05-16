package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Validator struct {
	mu      sync.RWMutex
	schemas map[string]*jsonSchema
}

type jsonSchema struct {
	Required []string          `json:"required"`
	Type     string            `json:"type"`
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

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if schema.Type == "object" {
		for _, field := range schema.Required {
			if _, ok := doc[field]; !ok {
				return fmt.Errorf("missing required field: %s", field)
			}
		}
	}

	return nil
}
