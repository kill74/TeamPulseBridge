package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Config contains runtime values for webhook signature validation.
type Config struct {
	Environment                       string
	Port                              string
	SlackSigningSecret                string
	GitHubWebhookSecret               string
	GitLabWebhookToken                string
	TeamsClientState                  string
	QueueBuffer                       int
	RequestTimeoutSec                 int
	RequireSecrets                    bool
	QueueBackend                      string
	PubSubProjectID                   string
	PubSubTopicID                     string
	AdminAuthEnabled                  bool
	AdminJWTIssuer                    string
	AdminJWTAudience                  string
	AdminJWTSecret                    string
	AdminAllowCIDRs                   []string
	TrustedProxyCIDRs                 []string
	RateLimitEnabled                  bool
	RateLimitRPM                      int
	AdminRateLimitRPM                 int
	DedupEnabled                      bool
	DedupTTLSeconds                   int
	FailedStoreEnabled                bool
	FailedStorePath                   string
	ReplayAuditEnabled                bool
	ReplayAuditPath                   string
	QueueBackpressureEnabled          bool
	QueueBackpressureSoftLimitPercent int
	QueueBackpressureHardLimitPercent int
	QueueFailureBudgetPercent         int
	QueueFailureBudgetWindow          int
	QueueFailureBudgetMinSamples      int
	QueueThrottleRetryAfterSec        int
}

func LoadFromEnv() Config {
	return Config{
		Environment:                       envOrDefault("ENVIRONMENT", "dev"),
		Port:                              envOrDefault("PORT", "8080"),
		SlackSigningSecret:                os.Getenv("SLACK_SIGNING_SECRET"),
		GitHubWebhookSecret:               os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitLabWebhookToken:                os.Getenv("GITLAB_WEBHOOK_TOKEN"),
		TeamsClientState:                  os.Getenv("TEAMS_CLIENT_STATE"),
		QueueBuffer:                       intOrDefault("QUEUE_BUFFER", 4096),
		RequestTimeoutSec:                 intOrDefault("REQUEST_TIMEOUT_SEC", 15),
		RequireSecrets:                    boolOrDefault("REQUIRE_SECRETS", true),
		QueueBackend:                      envOrDefault("QUEUE_BACKEND", "log"),
		PubSubProjectID:                   os.Getenv("PUBSUB_PROJECT_ID"),
		PubSubTopicID:                     os.Getenv("PUBSUB_TOPIC_ID"),
		AdminAuthEnabled:                  boolOrDefault("ADMIN_AUTH_ENABLED", false),
		AdminJWTIssuer:                    os.Getenv("ADMIN_JWT_ISSUER"),
		AdminJWTAudience:                  os.Getenv("ADMIN_JWT_AUDIENCE"),
		AdminJWTSecret:                    os.Getenv("ADMIN_JWT_SECRET"),
		AdminAllowCIDRs:                   splitCSVEnv("ADMIN_ALLOW_CIDRS"),
		TrustedProxyCIDRs:                 splitCSVEnv("TRUSTED_PROXY_CIDRS"),
		RateLimitEnabled:                  boolOrDefault("RATE_LIMIT_ENABLED", true),
		RateLimitRPM:                      intOrDefault("RATE_LIMIT_RPM", 300),
		AdminRateLimitRPM:                 intOrDefault("ADMIN_RATE_LIMIT_RPM", 60),
		DedupEnabled:                      boolOrDefault("DEDUP_ENABLED", true),
		DedupTTLSeconds:                   intOrDefault("DEDUP_TTL_SEC", 300),
		FailedStoreEnabled:                boolOrDefault("FAILED_EVENT_STORE_ENABLED", true),
		FailedStorePath:                   envOrDefault("FAILED_EVENT_STORE_PATH", "data/failed-events.jsonl"),
		ReplayAuditEnabled:                boolOrDefault("REPLAY_AUDIT_ENABLED", true),
		ReplayAuditPath:                   envOrDefault("REPLAY_AUDIT_PATH", "data/replay-audit.jsonl"),
		QueueBackpressureEnabled:          boolOrDefault("QUEUE_BACKPRESSURE_ENABLED", true),
		QueueBackpressureSoftLimitPercent: intOrDefault("QUEUE_BACKPRESSURE_SOFT_LIMIT_PERCENT", 70),
		QueueBackpressureHardLimitPercent: intOrDefault("QUEUE_BACKPRESSURE_HARD_LIMIT_PERCENT", 90),
		QueueFailureBudgetPercent:         intOrDefault("QUEUE_FAILURE_BUDGET_PERCENT", 15),
		QueueFailureBudgetWindow:          intOrDefault("QUEUE_FAILURE_BUDGET_WINDOW", 100),
		QueueFailureBudgetMinSamples:      intOrDefault("QUEUE_FAILURE_BUDGET_MIN_SAMPLES", 20),
		QueueThrottleRetryAfterSec:        intOrDefault("QUEUE_THROTTLE_RETRY_AFTER_SEC", 5),
	}
}

func (c Config) Validate() error {
	if c.Port == "" {
		return errors.New("PORT must not be empty")
	}
	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("PORT must be a valid TCP port (1-65535), got %q", c.Port)
	}
	if c.QueueBuffer <= 0 {
		return fmt.Errorf("QUEUE_BUFFER must be > 0, got %d", c.QueueBuffer)
	}
	if c.QueueBuffer > 1_000_000 {
		return fmt.Errorf("QUEUE_BUFFER is too high (%d); expected <= 1000000", c.QueueBuffer)
	}
	if c.RequestTimeoutSec <= 0 {
		return fmt.Errorf("REQUEST_TIMEOUT_SEC must be > 0, got %d", c.RequestTimeoutSec)
	}
	if c.RequestTimeoutSec > 300 {
		return fmt.Errorf("REQUEST_TIMEOUT_SEC is too high (%d); expected <= 300", c.RequestTimeoutSec)
	}
	if c.RateLimitEnabled {
		if c.RateLimitRPM < 10 || c.RateLimitRPM > 100000 {
			return fmt.Errorf("RATE_LIMIT_RPM must be between 10 and 100000, got %d", c.RateLimitRPM)
		}
		if c.AdminRateLimitRPM < 1 || c.AdminRateLimitRPM > c.RateLimitRPM {
			return fmt.Errorf("ADMIN_RATE_LIMIT_RPM must be between 1 and RATE_LIMIT_RPM (%d), got %d", c.RateLimitRPM, c.AdminRateLimitRPM)
		}
	}
	dedupTTL := c.DedupTTLSeconds
	if dedupTTL == 0 {
		dedupTTL = 300
	}
	if dedupTTL < 1 || dedupTTL > 86400 {
		return fmt.Errorf("DEDUP_TTL_SEC must be between 1 and 86400, got %d", c.DedupTTLSeconds)
	}
	if c.FailedStoreEnabled && strings.TrimSpace(c.FailedStorePath) == "" {
		return errors.New("FAILED_EVENT_STORE_PATH must not be empty when FAILED_EVENT_STORE_ENABLED=true")
	}
	if c.ReplayAuditEnabled && strings.TrimSpace(c.ReplayAuditPath) == "" {
		return errors.New("REPLAY_AUDIT_PATH must not be empty when REPLAY_AUDIT_ENABLED=true")
	}
	if c.QueueBackpressureEnabled {
		if c.QueueBackpressureSoftLimitPercent < 1 || c.QueueBackpressureSoftLimitPercent > 99 {
			return fmt.Errorf("QUEUE_BACKPRESSURE_SOFT_LIMIT_PERCENT must be between 1 and 99, got %d", c.QueueBackpressureSoftLimitPercent)
		}
		if c.QueueBackpressureHardLimitPercent < 1 || c.QueueBackpressureHardLimitPercent > 100 {
			return fmt.Errorf("QUEUE_BACKPRESSURE_HARD_LIMIT_PERCENT must be between 1 and 100, got %d", c.QueueBackpressureHardLimitPercent)
		}
		if c.QueueBackpressureHardLimitPercent <= c.QueueBackpressureSoftLimitPercent {
			return fmt.Errorf(
				"QUEUE_BACKPRESSURE_HARD_LIMIT_PERCENT (%d) must be greater than QUEUE_BACKPRESSURE_SOFT_LIMIT_PERCENT (%d)",
				c.QueueBackpressureHardLimitPercent,
				c.QueueBackpressureSoftLimitPercent,
			)
		}
		if c.QueueFailureBudgetPercent < 1 || c.QueueFailureBudgetPercent > 100 {
			return fmt.Errorf("QUEUE_FAILURE_BUDGET_PERCENT must be between 1 and 100, got %d", c.QueueFailureBudgetPercent)
		}
		if c.QueueFailureBudgetWindow < 1 || c.QueueFailureBudgetWindow > 100000 {
			return fmt.Errorf("QUEUE_FAILURE_BUDGET_WINDOW must be between 1 and 100000, got %d", c.QueueFailureBudgetWindow)
		}
		if c.QueueFailureBudgetMinSamples < 1 || c.QueueFailureBudgetMinSamples > c.QueueFailureBudgetWindow {
			return fmt.Errorf(
				"QUEUE_FAILURE_BUDGET_MIN_SAMPLES must be between 1 and QUEUE_FAILURE_BUDGET_WINDOW (%d), got %d",
				c.QueueFailureBudgetWindow,
				c.QueueFailureBudgetMinSamples,
			)
		}
		if c.QueueThrottleRetryAfterSec < 1 || c.QueueThrottleRetryAfterSec > 300 {
			return fmt.Errorf("QUEUE_THROTTLE_RETRY_AFTER_SEC must be between 1 and 300, got %d", c.QueueThrottleRetryAfterSec)
		}
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid TRUSTED_PROXY_CIDRS value %q: %w", cidr, err)
		}
	}
	if c.QueueBackend != "log" && c.QueueBackend != "pubsub" {
		return fmt.Errorf("QUEUE_BACKEND must be one of log|pubsub, got %q", c.QueueBackend)
	}
	if c.QueueBackend == "pubsub" {
		if c.PubSubProjectID == "" {
			return errors.New("PUBSUB_PROJECT_ID is required when QUEUE_BACKEND=pubsub")
		}
		if c.PubSubTopicID == "" {
			return errors.New("PUBSUB_TOPIC_ID is required when QUEUE_BACKEND=pubsub")
		}
		if strings.ContainsAny(c.PubSubProjectID, " \t\n\r") {
			return errors.New("PUBSUB_PROJECT_ID must not contain whitespace")
		}
		if strings.ContainsAny(c.PubSubTopicID, " \t\n\r") {
			return errors.New("PUBSUB_TOPIC_ID must not contain whitespace")
		}
	}
	if c.AdminAuthEnabled {
		missing := make([]string, 0, 3)
		if c.AdminJWTIssuer == "" {
			missing = append(missing, "ADMIN_JWT_ISSUER")
		}
		if c.AdminJWTAudience == "" {
			missing = append(missing, "ADMIN_JWT_AUDIENCE")
		}
		if c.AdminJWTSecret == "" {
			missing = append(missing, "ADMIN_JWT_SECRET")
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing admin auth values: %s", strings.Join(missing, ", "))
		}
		if len(c.AdminJWTSecret) < 32 || isWeakSecret(c.AdminJWTSecret) {
			return errors.New("ADMIN_JWT_SECRET must be at least 32 chars and not a weak/default value")
		}
		for _, cidr := range c.AdminAllowCIDRs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("invalid ADMIN_ALLOW_CIDRS value %q: %w", cidr, err)
			}
		}
		if !isNonProdEnvironment(c.Environment) && len(c.AdminAllowCIDRs) == 0 {
			return errors.New("ADMIN_ALLOW_CIDRS is required when ADMIN_AUTH_ENABLED=true in production-like environments")
		}
	}
	if !c.RequireSecrets {
		if !isNonProdEnvironment(c.Environment) {
			return fmt.Errorf("REQUIRE_SECRETS=false is only allowed in non-prod environments, got ENVIRONMENT=%q", c.Environment)
		}
		return nil
	}

	missing := make([]string, 0, 4)
	if c.SlackSigningSecret == "" {
		missing = append(missing, "SLACK_SIGNING_SECRET")
	}
	if c.GitHubWebhookSecret == "" {
		missing = append(missing, "GITHUB_WEBHOOK_SECRET")
	}
	if c.GitLabWebhookToken == "" {
		missing = append(missing, "GITLAB_WEBHOOK_TOKEN")
	}
	if c.TeamsClientState == "" {
		missing = append(missing, "TEAMS_CLIENT_STATE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required secrets: %s", strings.Join(missing, ", "))
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func boolOrDefault(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	if v == "1" || v == "true" || v == "yes" || v == "y" {
		return true
	}
	if v == "0" || v == "false" || v == "no" || v == "n" {
		return false
	}
	return fallback
}

func splitCSVEnv(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		candidate := strings.TrimSpace(p)
		if candidate != "" {
			out = append(out, candidate)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isNonProdEnvironment(env string) bool {
	v := strings.ToLower(strings.TrimSpace(env))
	if v == "" {
		return false
	}
	if strings.Contains(v, "dev") || strings.Contains(v, "test") || strings.Contains(v, "local") || strings.Contains(v, "ci") {
		return true
	}
	return v == "staging" || v == "sandbox"
}

func isWeakSecret(secret string) bool {
	v := strings.ToLower(strings.TrimSpace(secret))
	if v == "" {
		return true
	}
	weakValues := map[string]struct{}{
		"change-me": {},
		"changeme":  {},
		"secret":    {},
		"password":  {},
		"admin":     {},
		"test":      {},
		"default":   {},
	}
	if _, ok := weakValues[v]; ok {
		return true
	}
	if strings.Count(v, string(v[0])) == len(v) {
		return true
	}
	return false
}
