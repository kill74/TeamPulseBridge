package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidator_Validate_MissingRequiredField(t *testing.T) {
	tempDir := t.TempDir()

	schemaContent := `{
		"type": "object",
		"required": ["id", "name"]
	}`

	err := os.WriteFile(filepath.Join(tempDir, "testsource.json"), []byte(schemaContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test schema: %v", err)
	}

	validator, err := NewValidator(tempDir)
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	tests := []struct {
		name    string
		source  string
		body    []byte
		wantErr bool
		errMsg  string
	}{
		{
			name:    "all required fields present",
			source:  "testsource",
			body:    []byte(`{"id": 123, "name": "test"}`),
			wantErr: false,
		},
		{
			name:    "missing one required field",
			source:  "testsource",
			body:    []byte(`{"id": 123}`),
			wantErr: true,
			errMsg:  "missing required field: name",
		},
		{
			name:    "missing all required fields",
			source:  "testsource",
			body:    []byte(`{"description": "no required fields here"}`),
			wantErr: true,
			errMsg:  "missing required field: id",
		},
		{
			name:    "invalid JSON object",
			source:  "testsource",
			body:    []byte(`[]`),
			wantErr: true,
			errMsg:  "invalid JSON object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.source, tt.body)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() error = nil, wantErr %v", tt.wantErr)
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, wantErrMsg %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}
