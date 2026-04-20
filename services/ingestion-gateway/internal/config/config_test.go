package config

import "testing"

func TestValidateRejectsInvalidPort(t *testing.T) {
	cfg := Config{Port: "abc", QueueBuffer: 1, RequestTimeoutSec: 15, QueueBackend: "log", RequireSecrets: false, Environment: "local"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestValidateRejectsWeakAdminSecret(t *testing.T) {
	cfg := Config{
		Environment:         "prod",
		Port:                "8080",
		QueueBuffer:         100,
		RequestTimeoutSec:   15,
		QueueBackend:        "log",
		RequireSecrets:      true,
		SlackSigningSecret:  "ok",
		GitHubWebhookSecret: "ok",
		GitLabWebhookToken:  "ok",
		TeamsClientState:    "ok",
		AdminAuthEnabled:    true,
		AdminJWTIssuer:      "teampulse",
		AdminJWTAudience:    "ops",
		AdminJWTSecret:      "change-me",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected weak admin jwt secret error")
	}
}

func TestValidateRejectsRequireSecretsFalseInProd(t *testing.T) {
	cfg := Config{Environment: "prod", Port: "8080", QueueBuffer: 1, RequestTimeoutSec: 15, QueueBackend: "log", RequireSecrets: false}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for REQUIRE_SECRETS=false in prod")
	}
}

func TestValidateAllowsRequireSecretsFalseInIntegrationTest(t *testing.T) {
	cfg := Config{Environment: "integration-test", Port: "8080", QueueBuffer: 1, RequestTimeoutSec: 15, QueueBackend: "log", RequireSecrets: false}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected integration-test to allow REQUIRE_SECRETS=false, got %v", err)
	}
}

func TestValidateRejectsPubSubIDsWithWhitespace(t *testing.T) {
	cfg := Config{
		Environment:       "local",
		Port:              "8080",
		QueueBuffer:       100,
		RequestTimeoutSec: 15,
		QueueBackend:      "pubsub",
		PubSubProjectID:   "my project",
		PubSubTopicID:     "topic",
		RequireSecrets:    false,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected pubsub whitespace validation error")
	}
}

func TestValidateRequiresAdminCIDRsInProdWhenAdminAuthEnabled(t *testing.T) {
	cfg := Config{
		Environment:         "prod",
		Port:                "8080",
		QueueBuffer:         100,
		RequestTimeoutSec:   15,
		QueueBackend:        "log",
		RequireSecrets:      true,
		SlackSigningSecret:  "ok",
		GitHubWebhookSecret: "ok",
		GitLabWebhookToken:  "ok",
		TeamsClientState:    "ok",
		AdminAuthEnabled:    true,
		AdminJWTIssuer:      "teampulse",
		AdminJWTAudience:    "ops",
		AdminJWTSecret:      "this-is-a-very-strong-secret-with-32+",
		AdminAllowCIDRs:     nil,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected ADMIN_ALLOW_CIDRS validation error")
	}
}

func TestValidateRejectsInvalidAdminCIDR(t *testing.T) {
	cfg := Config{
		Environment:         "staging",
		Port:                "8080",
		QueueBuffer:         100,
		RequestTimeoutSec:   15,
		QueueBackend:        "log",
		RequireSecrets:      true,
		SlackSigningSecret:  "ok",
		GitHubWebhookSecret: "ok",
		GitLabWebhookToken:  "ok",
		TeamsClientState:    "ok",
		AdminAuthEnabled:    true,
		AdminJWTIssuer:      "teampulse",
		AdminJWTAudience:    "ops",
		AdminJWTSecret:      "this-is-a-very-strong-secret-with-32+",
		AdminAllowCIDRs:     []string{"invalid"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid admin CIDR error")
	}
}

func TestValidateRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	cfg := Config{
		Environment:       "local",
		Port:              "8080",
		QueueBuffer:       100,
		RequestTimeoutSec: 15,
		QueueBackend:      "log",
		RequireSecrets:    false,
		TrustedProxyCIDRs: []string{"not-a-cidr"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid TRUSTED_PROXY_CIDRS error")
	}
}

func TestValidateRejectsInvalidDedupTTL(t *testing.T) {
	cfg := Config{
		Environment:       "local",
		Port:              "8080",
		QueueBuffer:       100,
		RequestTimeoutSec: 15,
		QueueBackend:      "log",
		RequireSecrets:    false,
		DedupTTLSeconds:   999999,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid DEDUP_TTL_SEC error")
	}
}

func TestValidateRejectsEmptyFailedStorePathWhenEnabled(t *testing.T) {
	cfg := Config{
		Environment:        "local",
		Port:               "8080",
		QueueBuffer:        100,
		RequestTimeoutSec:  15,
		QueueBackend:       "log",
		RequireSecrets:     false,
		FailedStoreEnabled: true,
		FailedStorePath:    "  ",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected FAILED_EVENT_STORE_PATH validation error")
	}
}

func TestValidateRejectsEmptyReplayAuditPathWhenEnabled(t *testing.T) {
	cfg := Config{
		Environment:        "local",
		Port:               "8080",
		QueueBuffer:        100,
		RequestTimeoutSec:  15,
		QueueBackend:       "log",
		RequireSecrets:     false,
		ReplayAuditEnabled: true,
		ReplayAuditPath:    " ",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected REPLAY_AUDIT_PATH validation error")
	}
}

func TestValidateRejectsInvalidQueueBackpressureLimits(t *testing.T) {
	cfg := Config{
		Environment:                       "local",
		Port:                              "8080",
		QueueBuffer:                       100,
		RequestTimeoutSec:                 15,
		QueueBackend:                      "log",
		RequireSecrets:                    false,
		QueueBackpressureEnabled:          true,
		QueueBackpressureSoftLimitPercent: 90,
		QueueBackpressureHardLimitPercent: 80,
		QueueFailureBudgetPercent:         15,
		QueueFailureBudgetWindow:          100,
		QueueFailureBudgetMinSamples:      20,
		QueueThrottleRetryAfterSec:        5,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid backpressure limit error")
	}
}

func TestValidateRejectsQueueFailureBudgetMinSamplesAboveWindow(t *testing.T) {
	cfg := Config{
		Environment:                       "local",
		Port:                              "8080",
		QueueBuffer:                       100,
		RequestTimeoutSec:                 15,
		QueueBackend:                      "log",
		RequireSecrets:                    false,
		QueueBackpressureEnabled:          true,
		QueueBackpressureSoftLimitPercent: 70,
		QueueBackpressureHardLimitPercent: 90,
		QueueFailureBudgetPercent:         15,
		QueueFailureBudgetWindow:          10,
		QueueFailureBudgetMinSamples:      20,
		QueueThrottleRetryAfterSec:        5,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid failure budget sample window error")
	}
}
