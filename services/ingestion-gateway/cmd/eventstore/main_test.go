package main

import "testing"

func TestRuntimeConfigValidateRequiresCoreValues(t *testing.T) {
	cfg := runtimeConfig{
		MaxOutstandingMessages: 100,
		ReceiveGoroutines:      1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing config error")
	}
}

func TestRuntimeConfigValidateAcceptsDefaults(t *testing.T) {
	cfg := runtimeConfig{
		DatabaseURL:            "postgres://user:pass@localhost:5432/db?sslmode=disable",
		PubSubProjectID:        "test-project",
		PubSubSubscriptionID:   "webhook-events-store",
		MaxOutstandingMessages: 100,
		ReceiveGoroutines:      1,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeConfigValidateRejectsUnsafeReceiveSettings(t *testing.T) {
	cfg := runtimeConfig{
		DatabaseURL:            "postgres://user:pass@localhost:5432/db?sslmode=disable",
		PubSubProjectID:        "test-project",
		PubSubSubscriptionID:   "webhook-events-store",
		MaxOutstandingMessages: 0,
		ReceiveGoroutines:      1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected max outstanding validation error")
	}
}
