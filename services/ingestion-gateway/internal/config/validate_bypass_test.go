package config

import (
	"testing"
)

func baseNonProdConfig() Config {
	return Config{
		Environment:            "local",
		Port:                   "8080",
		QueueBuffer:            1,
		RequestTimeoutSec:      15,
		QueueBackend:           "log",
		RequireSecrets:         false,
		SourceRateLimitEnabled: false,
	}
}

func TestValidateChaosCheckedWhenSecretsNotRequired(t *testing.T) {
	cfg := baseNonProdConfig()
	cfg.ChaosEnabled = true
	cfg.ChaosErrorRate = 9.0 // invalid: must be within [0,1]
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected chaos validation error when REQUIRE_SECRETS=false, got nil")
	}
}

func TestValidateChaosLatencyOrderCheckedWhenSecretsNotRequired(t *testing.T) {
	cfg := baseNonProdConfig()
	cfg.ChaosEnabled = true
	cfg.ChaosErrorRate = 0.1
	cfg.ChaosLatencyRate = 0.1
	cfg.ChaosLatencyMinMs = 500
	cfg.ChaosLatencyMaxMs = 100
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected chaos latency order error when REQUIRE_SECRETS=false, got nil")
	}
}

func TestValidateSourceRateLimitCheckedWhenSecretsNotRequired(t *testing.T) {
	cfg := baseNonProdConfig()
	cfg.SourceRateLimitEnabled = true
	cfg.SourceRateLimitDefault = 0 // invalid: must be within [1,1000000]
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected source rate limit error when REQUIRE_SECRETS=false, got nil")
	}
}

func TestLoadFromMapDefaultsMatchEnvDefaults(t *testing.T) {
	cfg := LoadFromMap(map[string]string{})
	if cfg.QueueWorkers != 16 {
		t.Fatalf("QUEUE_WORKERS default = %d, want 16", cfg.QueueWorkers)
	}
	if !cfg.QueueBulkheadEnabled {
		t.Fatal("QUEUE_BULKHEAD_ENABLED default = false, want true")
	}
	if cfg.QueueBulkheadMaxSources != 8 {
		t.Fatalf("QUEUE_BULKHEAD_MAX_SOURCES default = %d, want 8", cfg.QueueBulkheadMaxSources)
	}
	if cfg.QueueThrottleRetryAfterSec != 2 {
		t.Fatalf("QUEUE_THROTTLE_RETRY_AFTER_SEC default = %d, want 2", cfg.QueueThrottleRetryAfterSec)
	}
	if cfg.PubSubPublishGoroutines != 16 {
		t.Fatalf("PUBSUB_PUBLISH_GOROUTINES default = %d, want 16", cfg.PubSubPublishGoroutines)
	}
	if cfg.PubSubMaxOutstandingMessages != 2000 {
		t.Fatalf("PUBSUB_MAX_OUTSTANDING_MESSAGES default = %d, want 2000", cfg.PubSubMaxOutstandingMessages)
	}
	if cfg.PubSubMaxOutstandingBytes != 104857600 {
		t.Fatalf("PUBSUB_MAX_OUTSTANDING_BYTES default = %d, want 104857600", cfg.PubSubMaxOutstandingBytes)
	}
	if cfg.PubSubFlowControlBehavior != "signal_error" {
		t.Fatalf("PUBSUB_FLOW_CONTROL_BEHAVIOR default = %q, want signal_error", cfg.PubSubFlowControlBehavior)
	}
	if cfg.RetryWorkers != 8 {
		t.Fatalf("RETRY_WORKERS default = %d, want 8", cfg.RetryWorkers)
	}
	if cfg.HTTPMaxInflight != 512 {
		t.Fatalf("HTTP_MAX_INFLIGHT default = %d, want 512", cfg.HTTPMaxInflight)
	}
}
