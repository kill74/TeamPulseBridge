package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_Validate(t *testing.T) {
	// Create a temporary schema directory
	tmpDir, err := os.MkdirTemp("", "schema-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a test schema
	schemaData := `{
		"type": "object",
		"required": ["event", "id"]
	}`
	err = os.WriteFile(filepath.Join(tmpDir, "github.json"), []byte(schemaData), 0644)
	require.NoError(t, err)

	v, err := NewValidator(tmpDir)
	require.NoError(t, err)

	tests := []struct {
		name    string
		source  string
		body    string
		wantErr bool
	}{
		{
			name:    "valid github payload",
			source:  "github",
			body:    `{"event":"push", "id":"123"}`,
			wantErr: false,
		},
		{
			name:    "missing required field",
			source:  "github",
			body:    `{"event":"push"}`,
			wantErr: true,
		},
		{
			name:    "invalid json",
			source:  "github",
			body:    `{"event":"push",`,
			wantErr: true,
		},
		{
			name:    "not an object",
			source:  "github",
			body:    `[1, 2, 3]`,
			wantErr: true,
		},
		{
			name:    "no schema for source",
			source:  "slack",
			body:    `{"anything":true}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.source, []byte(tt.body))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_LoadSchemas_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "schema-empty-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	v, err := NewValidator(tmpDir)
	assert.Error(t, err)
	assert.Nil(t, v)
}

func TestValidator_NoSchemaPath(t *testing.T) {
	v, err := NewValidator("")
	assert.NoError(t, err)
	assert.NotNil(t, v)
	
	err = v.Validate("any", []byte(`{"a":1}`))
	assert.NoError(t, err)
}
