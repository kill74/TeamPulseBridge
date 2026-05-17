package schema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_LoadSchemas_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()

	invalidJSON := []byte(`{ "type": "object", "properties": { "id": { "type": "string" }`) // missing closing brace
	err := os.WriteFile(filepath.Join(tempDir, "invalid.json"), invalidJSON, 0644)
	require.NoError(t, err)

	v, err := NewValidator(tempDir)
	assert.Error(t, err)
	assert.Nil(t, v)
	assert.Contains(t, err.Error(), "parse schema file")
	assert.Contains(t, err.Error(), "invalid.json")
}

func TestValidator_LoadSchemas_ReadError(t *testing.T) {
	tempDir := t.TempDir()

	// Create a directory with .json extension so it's matched by glob but can't be read as a file
	err := os.Mkdir(filepath.Join(tempDir, "unreadable.json"), 0755)
	require.NoError(t, err)

	v, err := NewValidator(tempDir)
	assert.Error(t, err)
	assert.Nil(t, v)
	assert.Contains(t, err.Error(), "read schema file")
	assert.Contains(t, err.Error(), "unreadable.json")
}
