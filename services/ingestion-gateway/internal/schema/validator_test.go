package schema

import (
	"testing"
)

func TestValidator_Validate(t *testing.T) {
	v := &Validator{
		schemas: map[string]*jsonSchema{
			"non-object": {
				Type: "array",
			},
			"object-schema": {
				Type:     "object",
				Required: []string{"id", "name"},
			},
		},
	}

	tests := []struct {
		name    string
		source  string
		body    []byte
		wantErr bool
	}{
		{
			name:    "schema does not exist",
			source:  "unknown",
			body:    []byte(`{"foo": "bar"}`),
			wantErr: false,
		},
		{
			name:    "non-object schema, valid json",
			source:  "non-object",
			body:    []byte(`[1, 2, 3]`),
			wantErr: false,
		},
		{
			name:    "non-object schema, invalid json",
			source:  "non-object",
			body:    []byte(`[1, 2, 3`),
			wantErr: true,
		},
		{
			name:    "object schema, valid json with all required fields",
			source:  "object-schema",
			body:    []byte(`{"id": 1, "name": "foo", "extra": true}`),
			wantErr: false,
		},
		{
			name:    "object schema, valid json missing required field",
			source:  "object-schema",
			body:    []byte(`{"id": 1, "extra": true}`),
			wantErr: true,
		},
		{
			name:    "object schema, invalid json",
			source:  "object-schema",
			body:    []byte(`{"id": 1, "name": "foo"`),
			wantErr: true,
		},
		{
			name:    "object schema, not an object json",
			source:  "object-schema",
			body:    []byte(`[1, 2, 3]`),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.source, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
