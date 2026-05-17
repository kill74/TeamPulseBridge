package schema

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidator(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		v, err := NewValidator("")
		require.NoError(t, err)
		require.NotNil(t, v)
		assert.Empty(t, v.schemas)
	})

	t.Run("valid path", func(t *testing.T) {
		path := filepath.Join("testdata", "good")
		v, err := NewValidator(path)
		require.NoError(t, err)
		require.NotNil(t, v)
		assert.Len(t, v.schemas, 2)
		assert.Contains(t, v.schemas, "valid")
		assert.Contains(t, v.schemas, "not_object")
	})

	t.Run("missing directory", func(t *testing.T) {
		path := filepath.Join("testdata", "does-not-exist")
		v, err := NewValidator(path)
		require.Error(t, err)
		require.Nil(t, v)
		assert.Contains(t, err.Error(), "no schema files found")
	})

	t.Run("invalid schema", func(t *testing.T) {
		path := filepath.Join("testdata", "bad")
		v, err := NewValidator(path)
		require.Error(t, err)
		require.Nil(t, v)
		assert.Contains(t, err.Error(), "parse schema file")
	})
}

func TestValidator_Validate(t *testing.T) {
	path := filepath.Join("testdata", "good")
	v, err := NewValidator(path)
	require.NoError(t, err)

	t.Run("unknown source", func(t *testing.T) {
		err := v.Validate("unknown", []byte(`{"id": 1}`))
		assert.NoError(t, err)
	})

	t.Run("not object schema type with valid json", func(t *testing.T) {
		err := v.Validate("not_object", []byte(`[1, 2, 3]`))
		assert.NoError(t, err)
	})

	t.Run("not object schema type with invalid json", func(t *testing.T) {
		err := v.Validate("not_object", []byte(`[1, 2, 3`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("invalid json for object schema", func(t *testing.T) {
		err := v.Validate("valid", []byte(`{"id": 1, "name": "foo"`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON")
	})

	t.Run("valid object schema but wrong root structure", func(t *testing.T) {
		err := v.Validate("valid", []byte(`[{"id": 1, "name": "foo"}]`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON object")
	})

	t.Run("missing required field", func(t *testing.T) {
		err := v.Validate("valid", []byte(`{"id": 1}`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field: name")
	})

	t.Run("all required fields present", func(t *testing.T) {
		err := v.Validate("valid", []byte(`{"id": 1, "name": "foo", "extra": true}`))
		assert.NoError(t, err)
	})

	t.Run("all required fields present, nested object", func(t *testing.T) {
		err := v.Validate("valid", []byte(`{"id": 1, "name": "foo", "extra": {"a": 1}}`))
		assert.NoError(t, err)
	})

	t.Run("all required fields present, empty extra string", func(t *testing.T) {
		err := v.Validate("valid", []byte(`{"id": 1, "name": "foo", "extra": ""}`))
		assert.NoError(t, err)
	})

	t.Run("all required fields present, nested object, extra whitespace", func(t *testing.T) {
		err := v.Validate("valid", []byte(` { "id" : 1, "name" : "foo", "extra" : { "a" : 1 } } `))
		assert.NoError(t, err)
	})
}

func TestTopLevelObjectFields(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		want    map[string]struct{}
		wantErr string
	}{
		{
			name: "valid simple",
			body: []byte(`{"a": 1, "b": "two"}`),
			want: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name: "valid nested",
			body: []byte(`{"a": {"c": 1}, "b": "two"}`),
			want: map[string]struct{}{"a": {}, "b": {}},
		},
		{
			name:    "invalid json array",
			body:    []byte(`[{"a": 1}]`),
			wantErr: "invalid JSON object",
		},
		{
			name:    "invalid json string",
			body:    []byte(`"string"`),
			wantErr: "invalid JSON object",
		},
		{
			name:    "invalid json syntax",
			body:    []byte(`{"a": 1`),
			wantErr: "invalid JSON object",
		},
		{
			name:    "invalid key type",
			body:    []byte(`{1: 2}`), // Not valid JSON technically, depends on parser
			wantErr: "invalid JSON object key",
		},
		{
			name:    "trailing data",
			body:    []byte(`{"a": 1} {"b": 2}`),
			wantErr: "invalid JSON: trailing data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := topLevelObjectFields(tt.body)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
// Test suite for validator.go
