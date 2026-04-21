package queue

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"teampulsebridge/services/ingestion-gateway/internal/testhelpers/fixturecatalog"
)

type schemaDocument struct {
	Type                 string                  `json:"type"`
	AdditionalProperties bool                    `json:"additionalProperties"`
	Required             []string                `json:"required"`
	Properties           map[string]schemaObject `json:"properties"`
}

type schemaObject struct {
	Type                 string        `json:"type"`
	Const                interface{}   `json:"const"`
	MinLength            *int          `json:"minLength"`
	Minimum              *float64      `json:"minimum"`
	Format               string        `json:"format"`
	AdditionalProperties *schemaObject `json:"additionalProperties"`
}

func TestRawWebhookEnvelopeSchema_CatalogFixturesStayCompatible(t *testing.T) {
	catalog, err := fixturecatalog.Load()
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}
	if catalog.EnvelopeSchema.Name != "raw-webhook-envelope" || catalog.EnvelopeSchema.Version != 1 {
		t.Fatalf("unexpected envelope schema reference: %+v", catalog.EnvelopeSchema)
	}
	schema, err := loadSchema(catalog.EnvelopeSchemaPath())
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	for _, tc := range catalog.PublishedFixtures() {
		t.Run(tc.ID, func(t *testing.T) {
			body, err := fixturecatalog.ReadFixture(tc)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			payload, err := json.Marshal(pubSubEnvelope{
				Source: tc.Provider,
				Headers: map[string]string{
					"Content-Type": "application/json",
					"User-Agent":   "contract-test/" + tc.ID,
				},
				Body:        body,
				ReceivedAt:  time.Now().UTC(),
				Schema:      "raw-webhook-envelope",
				SchemaValue: 1,
			})
			if err != nil {
				t.Fatalf("marshal envelope: %v", err)
			}

			if err := validateAgainstSchema(schema, payload); err != nil {
				t.Fatalf("schema validation failed: %v", err)
			}
		})
	}
}

func TestRawWebhookEnvelopeSchema_RejectsDrift(t *testing.T) {
	catalog, err := fixturecatalog.Load()
	if err != nil {
		t.Fatalf("load fixture catalog: %v", err)
	}
	schema, err := loadSchema(catalog.EnvelopeSchemaPath())
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	invalid := []byte(`{
		"source": "github",
		"headers": {"Content-Type":"application/json"},
		"body": {"action":"opened"},
		"received_at": "not-a-timestamp",
		"schema": "raw-webhook-envelope",
		"schema_value": 0,
		"unexpected": true
	}`)

	if err := validateAgainstSchema(schema, invalid); err == nil {
		t.Fatal("expected schema validation error for drift payload")
	}
}

func loadSchema(path string) (schemaDocument, error) {
	var schema schemaDocument
	raw, err := os.ReadFile(path)
	if err != nil {
		return schema, err
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return schema, err
	}
	return schema, nil
}

func validateAgainstSchema(schema schemaDocument, payload []byte) error {
	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return fmt.Errorf("invalid envelope json: %w", err)
	}

	if schema.Type != "object" {
		return fmt.Errorf("schema type must be object, got %q", schema.Type)
	}
	if schema.AdditionalProperties {
		return fmt.Errorf("schema additionalProperties must be false")
	}

	for _, key := range schema.Required {
		if _, ok := doc[key]; !ok {
			return fmt.Errorf("missing required property %q", key)
		}
	}

	for key := range doc {
		if _, ok := schema.Properties[key]; !ok {
			return fmt.Errorf("unexpected property %q", key)
		}
	}

	for name, rule := range schema.Properties {
		value, ok := doc[name]
		if !ok {
			continue
		}
		if err := validateRule(name, rule, value); err != nil {
			return err
		}
	}

	return nil
}

func validateRule(name string, rule schemaObject, value interface{}) error {
	switch rule.Type {
	case "string":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", name)
		}
		if rule.MinLength != nil && len(s) < *rule.MinLength {
			return fmt.Errorf("%s must have minLength=%d", name, *rule.MinLength)
		}
		if rule.Format == "date-time" {
			if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
				return fmt.Errorf("%s must be RFC3339 date-time: %w", name, err)
			}
		}
	case "integer":
		n, ok := value.(float64)
		if !ok || math.Trunc(n) != n {
			return fmt.Errorf("%s must be an integer", name)
		}
		if rule.Minimum != nil && n < *rule.Minimum {
			return fmt.Errorf("%s must be >= %.0f", name, *rule.Minimum)
		}
	case "object":
		obj, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s must be an object", name)
		}
		if rule.AdditionalProperties != nil {
			for k, v := range obj {
				if err := validateRule(name+"."+k, *rule.AdditionalProperties, v); err != nil {
					return err
				}
			}
		}
	}

	if rule.Const != nil {
		if value != rule.Const {
			return fmt.Errorf("%s must equal %v", name, rule.Const)
		}
	}
	return nil
}
