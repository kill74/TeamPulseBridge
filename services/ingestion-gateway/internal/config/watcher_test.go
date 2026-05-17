package config

import (
	"os"
	"testing"
)

func TestParseConfig(t *testing.T) {
	// Keys we expect the ParseConfig to modify
	keysToTest := []string{"ENVIRONMENT", "PORT", "QUEUE_BACKEND"}

	// Backup and cleanup environment variables
	for _, k := range keysToTest {
		key := k
		val, exists := os.LookupEnv(key)
		if exists {
			t.Cleanup(func() { os.Setenv(key, val) })
		} else {
			t.Cleanup(func() { os.Unsetenv(key) })
		}
	}

	mockConfig := `
# This is a comment
ENVIRONMENT=test-env

# Empty lines should be ignored

PORT=9999
QUEUE_BACKEND=memory
INVALID_LINE_WITHOUT_EQUALS
   # Indented comment
`

	cfg, err := ParseConfig([]byte(mockConfig))
	if err != nil {
		t.Fatalf("unexpected error parsing config: %v", err)
	}

	if cfg.Environment != "test-env" {
		t.Errorf("expected Environment to be 'test-env', got '%s'", cfg.Environment)
	}

	if cfg.Port != "9999" {
		t.Errorf("expected Port to be '9999', got '%s'", cfg.Port)
	}

	if cfg.QueueBackend != "memory" {
		t.Errorf("expected QueueBackend to be 'memory', got '%s'", cfg.QueueBackend)
	}
}
