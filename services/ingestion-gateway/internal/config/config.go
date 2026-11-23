package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
)

// Config contains runtime values for webhook signature validation.
//
// Performance tuning notes (see docs/performance.md):
//   - QUEUE_WORKERS defaults to 16 so Pub/Sub publish does not serialize on 1 worker.
//   - QUEUE_BULKHEAD_ENABLED defaults to true to isolate hot sources.
//   - Pub/Sub flow-control defaults to signal_error with sane outstanding limits
//     to fail fast instead of OOMing under spikes.
//   - PII scrubbing runs in queue workers (PII_SCRUB_ASYNC=true) to keep p50 low.
type Config struct {
	Environment                       string
	Port                              string
	SlackSigningSecret                string
	GitHubWebhookSecret               string
	GitLabWebhookToken                string
	TeamsClientState                  string
	QueueBuffer                       int
	QueueWorkers                      int
	QueueBulkheadEnabled              bool
	QueueBulkheadBufferPerSource      int
	QueueBulkheadMaxSources           int
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
	RedisAddr                         string
	RedisPassword                     string
	RedisDB                           int
	RedisPoolSize                     int
	RedisMinIdleConns                 int
	RedisIOTimeoutMs                  int
	RedisClusterAddrs                 []string
	RateLimitCombined                 bool
	DedupRedisPrefix                  string
	FailedStoreEnabled                bool
	FailedStorePath                   string
	DatabaseURL                       string
	ReplayAuditEnabled                bool
	ReplayAuditPath                   string
	SecurityAuditEnabled              bool
	SecurityAuditPath                 string
	SecurityAuditRetentionDays        int
	QueueBackpressureEnabled          bool
	QueueBackpressureSoftLimitPercent int
	QueueBackpressureHardLimitPercent int
	QueueFailureBudgetPercent         int
	QueueFailureBudgetWindow          int
	QueueFailureBudgetMinSamples      int
	QueueThrottleRetryAfterSec        int
	QueueBatchSize                    int
	QueueBatchFlushMs                 int
	SourceRateLimitEnabled            bool
	SourceRateLimits                  map[string]int
	SourceRateLimitDefault            int
	SchemaValidationEnabled           bool
	SchemaPath                        string
	RetryEnabled                      bool
	RetryMaxAttempts                  int
	RetryIntervalSec                  int
	RetryWorkers                      int
	PubSubPublishTimeoutSec           int
	PubSubPublishGoroutines           int
	PubSubMaxOutstandingMessages      int
	PubSubMaxOutstandingBytes         int
	PubSubFlowControlBehavior         string
	PubSubBatchDelayMs                int
	PubSubBatchCountThreshold         int
	PubSubBatchByteThreshold          int
	CircuitBreakerThreshold           int
	CircuitBreakerRecoverySec         int
	RateLimitBackend                  string
	RateLimitRedisPrefix              string
	PIIScrubbingEnabled               bool
	PIIScrubAsync                     bool
	PIIMaxScrubBytes                  int
	LogLevel                          string
	HTTPMaxInflight                   int
	HTTPMaxHeaderBytes                int
	HTTPH2CEnabled                    bool
	OTELTracesSamplerRatio            float64
	ChaosEnabled                      bool
	ChaosErrorRate                    float64
	ChaosLatencyRate                  float64
	ChaosLatencyMinMs                 int
	ChaosLatencyMaxMs                 int
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
		QueueWorkers:                      intOrDefault("QUEUE_WORKERS", 16),
		QueueBulkheadEnabled:              boolOrDefault("QUEUE_BULKHEAD_ENABLED", true),
		QueueBulkheadBufferPerSource:      intOrDefault("QUEUE_BULKHEAD_BUFFER_PER_SOURCE", 1024),
		QueueBulkheadMaxSources:           intOrDefault("QUEUE_BULKHEAD_MAX_SOURCES", 8),
		QueueBatchSize:                    intOrDefault("QUEUE_BATCH_SIZE", 100),
		QueueBatchFlushMs:                 intOrDefault("QUEUE_BATCH_FLUSH_MS", 50),
		RequestTimeoutSec:                 intOrDefault("REQUEST_TIMEOUT_SEC", 15),
		RequireSecrets:                    boolOrDefault("REQUIRE_SECRETS", true),
		QueueBackend:                      envOrDefault("QUEUE_BACKEND", "log"),
		PubSubProjectID:                   os.Getenv("PUBSUB_PROJECT_ID"),
		PubSubTopicID:                     os.Getenv("PUBSUB_TOPIC_ID"),
		AdminAuthEnabled:                  boolOrDefault("ADMIN_AUTH_ENABLED", true),
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
		RedisAddr:                         os.Getenv("REDIS_ADDR"),
		RedisPassword:                     os.Getenv("REDIS_PASSWORD"),
		RedisDB:                           intOrDefault("REDIS_DB", 0),
		RedisPoolSize:                     intOrDefault("REDIS_POOL_SIZE", 32),
		RedisMinIdleConns:                 intOrDefault("REDIS_MIN_IDLE_CONNS", 8),
		RedisIOTimeoutMs:                  intOrDefault("REDIS_IO_TIMEOUT_MS", 100),
		RedisClusterAddrs:                 splitCSVEnv("REDIS_CLUSTER_ADDRS"),
		RateLimitCombined:                 boolOrDefault("RATE_LIMIT_COMBINED", false),
		DedupRedisPrefix:                  envOrDefault("DEDUP_REDIS_PREFIX", "webhook_dedup"),
		FailedStoreEnabled:                boolOrDefault("FAILED_EVENT_STORE_ENABLED", true),
		FailedStorePath:                   envOrDefault("FAILED_EVENT_STORE_PATH", "data/failed-events.jsonl"),
		DatabaseURL:                       os.Getenv("DATABASE_URL"),
		ReplayAuditEnabled:                boolOrDefault("REPLAY_AUDIT_ENABLED", true),
		ReplayAuditPath:                   envOrDefault("REPLAY_AUDIT_PATH", "data/replay-audit.jsonl"),
		SecurityAuditEnabled:              boolOrDefault("SECURITY_AUDIT_ENABLED", true),
		SecurityAuditPath:                 envOrDefault("SECURITY_AUDIT_PATH", "data/security-audit.jsonl"),
		SecurityAuditRetentionDays:        intOrDefault("SECURITY_AUDIT_RETENTION_DAYS", 30),
		QueueBackpressureEnabled:          boolOrDefault("QUEUE_BACKPRESSURE_ENABLED", true),
		QueueBackpressureSoftLimitPercent: intOrDefault("QUEUE_BACKPRESSURE_SOFT_LIMIT_PERCENT", 70),
		QueueBackpressureHardLimitPercent: intOrDefault("QUEUE_BACKPRESSURE_HARD_LIMIT_PERCENT", 90),
		QueueFailureBudgetPercent:         intOrDefault("QUEUE_FAILURE_BUDGET_PERCENT", 15),
		QueueFailureBudgetWindow:          intOrDefault("QUEUE_FAILURE_BUDGET_WINDOW", 100),
		QueueFailureBudgetMinSamples:      intOrDefault("QUEUE_FAILURE_BUDGET_MIN_SAMPLES", 20),
		QueueThrottleRetryAfterSec:        intOrDefault("QUEUE_THROTTLE_RETRY_AFTER_SEC", 2),
		SourceRateLimitEnabled:            boolOrDefault("SOURCE_RATE_LIMIT_ENABLED", true),
		SourceRateLimits:                  parseSourceRateLimits(os.Getenv("SOURCE_RATE_LIMITS")),
		SourceRateLimitDefault:            intOrDefault("SOURCE_RATE_LIMIT_DEFAULT", 100),
		SchemaValidationEnabled:           boolOrDefault("SCHEMA_VALIDATION_ENABLED", true),
		SchemaPath:                        envOrDefault("SCHEMA_PATH", "internal/schema/schemas"),
		RetryEnabled:                      boolOrDefault("RETRY_ENABLED", false),
		RetryMaxAttempts:                  intOrDefault("RETRY_MAX_ATTEMPTS", 3),
		RetryIntervalSec:                  intOrDefault("RETRY_INTERVAL_SEC", 10),
		RetryWorkers:                      intOrDefault("RETRY_WORKERS", 8),
		PubSubPublishTimeoutSec:           intOrDefault("PUBSUB_PUBLISH_TIMEOUT_SEC", 5),
		PubSubPublishGoroutines:           intOrDefault("PUBSUB_PUBLISH_GOROUTINES", 16),
		PubSubMaxOutstandingMessages:      intOrDefault("PUBSUB_MAX_OUTSTANDING_MESSAGES", 2000),
		PubSubMaxOutstandingBytes:         intOrDefault("PUBSUB_MAX_OUTSTANDING_BYTES", 104857600),
		PubSubFlowControlBehavior:         envOrDefault("PUBSUB_FLOW_CONTROL_BEHAVIOR", "signal_error"),
		PubSubBatchDelayMs:                intOrDefault("PUBSUB_BATCH_DELAY_MS", 20),
		PubSubBatchCountThreshold:         intOrDefault("PUBSUB_BATCH_COUNT_THRESHOLD", 500),
		PubSubBatchByteThreshold:          intOrDefault("PUBSUB_BATCH_BYTE_THRESHOLD", 1048576),
		CircuitBreakerThreshold:           intOrDefault("CIRCUIT_BREAKER_THRESHOLD", 25),
		CircuitBreakerRecoverySec:         intOrDefault("CIRCUIT_BREAKER_RECOVERY_SEC", 8),
		RateLimitBackend:                  envOrDefault("RATE_LIMIT_BACKEND", "memory"),
		RateLimitRedisPrefix:              envOrDefault("RATE_LIMIT_REDIS_PREFIX", "rate_limit"),
		PIIScrubbingEnabled:               boolOrDefault("PII_SCRUBBING_ENABLED", false),
		PIIScrubAsync:                     boolOrDefault("PII_SCRUB_ASYNC", true),
		PIIMaxScrubBytes:                  intOrDefault("PII_MAX_SCRUB_BYTES", 262144),
		LogLevel:                          envOrDefault("LOG_LEVEL", "info"),
		HTTPMaxInflight:                   intOrDefault("HTTP_MAX_INFLIGHT", 512),
		HTTPMaxHeaderBytes:                intOrDefault("HTTP_MAX_HEADER_BYTES", 8192),
		HTTPH2CEnabled:                    boolOrDefault("HTTP_H2C_ENABLED", false),
		OTELTracesSamplerRatio:            floatOrDefault("OTEL_TRACES_SAMPLER_RATIO", 0.02),
		ChaosEnabled:                      boolOrDefault("CHAOS_ENABLED", false),
		ChaosErrorRate:                    floatOrDefault("CHAOS_ERROR_RATE", 0.0),
		ChaosLatencyRate:                  floatOrDefault("CHAOS_LATENCY_RATE", 0.0),
		ChaosLatencyMinMs:                 intOrDefault("CHAOS_LATENCY_MIN_MS", 100),
		ChaosLatencyMaxMs:                 intOrDefault("CHAOS_LATENCY_MAX_MS", 500),
	}
}

func floatOrDefault(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
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
	queueWorkers := c.QueueWorkers
	if queueWorkers == 0 {
		queueWorkers = 16
	}
	if queueWorkers < 1 || queueWorkers > 1024 {
		return fmt.Errorf("QUEUE_WORKERS must be between 1 and 1024, got %d", queueWorkers)
	}
	bulkheadBufferPerSource := c.QueueBulkheadBufferPerSource
	if bulkheadBufferPerSource == 0 {
		bulkheadBufferPerSource = 1024
	}
	if bulkheadBufferPerSource < 1 || bulkheadBufferPerSource > 1_000_000 {
		return fmt.Errorf("QUEUE_BULKHEAD_BUFFER_PER_SOURCE must be between 1 and 1000000, got %d", bulkheadBufferPerSource)
	}
	if c.QueueBulkheadMaxSources < 0 || c.QueueBulkheadMaxSources > 64 {
		return fmt.Errorf("QUEUE_BULKHEAD_MAX_SOURCES must be between 0 and 64, got %d", c.QueueBulkheadMaxSources)
	}
	if c.QueueBatchSize < 0 || c.QueueBatchSize > 10000 {
		return fmt.Errorf("QUEUE_BATCH_SIZE must be between 0 and 10000, got %d", c.QueueBatchSize)
	}
	if c.QueueBatchFlushMs < 0 || c.QueueBatchFlushMs > 5000 {
		return fmt.Errorf("QUEUE_BATCH_FLUSH_MS must be between 0 and 5000, got %d", c.QueueBatchFlushMs)
	}
	if c.RetryWorkers < 0 || c.RetryWorkers > 64 {
		return fmt.Errorf("RETRY_WORKERS must be between 0 and 64, got %d", c.RetryWorkers)
	}
	if c.CircuitBreakerThreshold < 0 || c.CircuitBreakerThreshold > 1000 {
		return fmt.Errorf("CIRCUIT_BREAKER_THRESHOLD must be between 0 and 1000, got %d", c.CircuitBreakerThreshold)
	}
	if c.CircuitBreakerRecoverySec < 0 || c.CircuitBreakerRecoverySec > 300 {
		return fmt.Errorf("CIRCUIT_BREAKER_RECOVERY_SEC must be between 0 and 300, got %d", c.CircuitBreakerRecoverySec)
	}
	if c.RedisPoolSize < 0 || c.RedisPoolSize > 512 {
		return fmt.Errorf("REDIS_POOL_SIZE must be between 0 and 512, got %d", c.RedisPoolSize)
	}
	if c.RedisMinIdleConns < 0 || c.RedisMinIdleConns > 512 {
		return fmt.Errorf("REDIS_MIN_IDLE_CONNS must be between 0 and 512, got %d", c.RedisMinIdleConns)
	}
	if c.RedisIOTimeoutMs < 0 || c.RedisIOTimeoutMs > 5000 {
		return fmt.Errorf("REDIS_IO_TIMEOUT_MS must be between 0 and 5000, got %d", c.RedisIOTimeoutMs)
	}
	if c.PIIMaxScrubBytes < 0 || c.PIIMaxScrubBytes > 4_194_304 {
		return fmt.Errorf("PII_MAX_SCRUB_BYTES must be between 0 and 4194304, got %d", c.PIIMaxScrubBytes)
	}
	if c.HTTPMaxInflight < 0 || c.HTTPMaxInflight > 100000 {
		return fmt.Errorf("HTTP_MAX_INFLIGHT must be between 0 and 100000, got %d", c.HTTPMaxInflight)
	}
	if c.HTTPMaxHeaderBytes < 0 || c.HTTPMaxHeaderBytes > 1_048_576 {
		return fmt.Errorf("HTTP_MAX_HEADER_BYTES must be between 0 and 1048576, got %d", c.HTTPMaxHeaderBytes)
	}
	if c.OTELTracesSamplerRatio < 0 || c.OTELTracesSamplerRatio > 1 {
		return fmt.Errorf("OTEL_TRACES_SAMPLER_RATIO must be between 0 and 1, got %f", c.OTELTracesSamplerRatio)
	}
	if c.PubSubBatchDelayMs < 0 || c.PubSubBatchDelayMs > 1000 {
		return fmt.Errorf("PUBSUB_BATCH_DELAY_MS must be between 0 and 1000, got %d", c.PubSubBatchDelayMs)
	}
	if c.PubSubBatchCountThreshold < 0 || c.PubSubBatchCountThreshold > 100000 {
		return fmt.Errorf("PUBSUB_BATCH_COUNT_THRESHOLD must be between 0 and 100000, got %d", c.PubSubBatchCountThreshold)
	}
	if c.PubSubBatchByteThreshold < 0 || c.PubSubBatchByteThreshold > 100_000_000 {
		return fmt.Errorf("PUBSUB_BATCH_BYTE_THRESHOLD must be between 0 and 100000000, got %d", c.PubSubBatchByteThreshold)
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
	if c.SecurityAuditEnabled {
		if strings.TrimSpace(c.SecurityAuditPath) == "" {
			return errors.New("SECURITY_AUDIT_PATH must not be empty when SECURITY_AUDIT_ENABLED=true")
		}
		if c.SecurityAuditRetentionDays < 1 || c.SecurityAuditRetentionDays > 3650 {
			return fmt.Errorf("SECURITY_AUDIT_RETENTION_DAYS must be between 1 and 3650, got %d", c.SecurityAuditRetentionDays)
		}
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
	rateLimitBackend := strings.TrimSpace(c.RateLimitBackend)
	if rateLimitBackend == "" {
		rateLimitBackend = "memory"
	}
	if rateLimitBackend != "memory" && rateLimitBackend != "redis" {
		return fmt.Errorf("RATE_LIMIT_BACKEND must be one of memory|redis, got %q", c.RateLimitBackend)
	}
	if rateLimitBackend == "redis" && strings.TrimSpace(c.RedisAddr) == "" {
		return errors.New("REDIS_ADDR is required when RATE_LIMIT_BACKEND=redis")
	}
	if strings.TrimSpace(c.RateLimitRedisPrefix) == "" && c.RateLimitRedisPrefix != "" {
		return errors.New("RATE_LIMIT_REDIS_PREFIX must not be whitespace-only")
	}
	pubsubPublishTimeoutSec := c.PubSubPublishTimeoutSec
	if pubsubPublishTimeoutSec == 0 {
		pubsubPublishTimeoutSec = 5
	}
	if pubsubPublishTimeoutSec < 1 || pubsubPublishTimeoutSec > 300 {
		return fmt.Errorf("PUBSUB_PUBLISH_TIMEOUT_SEC must be between 1 and 300, got %d", c.PubSubPublishTimeoutSec)
	}
	if c.PubSubPublishGoroutines < 0 || c.PubSubPublishGoroutines > 1024 {
		return fmt.Errorf("PUBSUB_PUBLISH_GOROUTINES must be between 0 and 1024, got %d", c.PubSubPublishGoroutines)
	}
	if c.PubSubMaxOutstandingMessages < 0 || c.PubSubMaxOutstandingMessages > 10_000_000 {
		return fmt.Errorf("PUBSUB_MAX_OUTSTANDING_MESSAGES must be between 0 and 10000000, got %d", c.PubSubMaxOutstandingMessages)
	}
	if c.PubSubMaxOutstandingBytes < 0 {
		return fmt.Errorf("PUBSUB_MAX_OUTSTANDING_BYTES must be >= 0, got %d", c.PubSubMaxOutstandingBytes)
	}
	pubsubFlowControlBehavior := strings.TrimSpace(c.PubSubFlowControlBehavior)
	if pubsubFlowControlBehavior == "" {
		pubsubFlowControlBehavior = "signal_error"
	}
	if pubsubFlowControlBehavior != "ignore" && pubsubFlowControlBehavior != "block" && pubsubFlowControlBehavior != "signal_error" {
		return fmt.Errorf("PUBSUB_FLOW_CONTROL_BEHAVIOR must be one of ignore|block|signal_error, got %q", c.PubSubFlowControlBehavior)
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
		if !IsNonProdEnvironment(c.Environment) && len(c.AdminAllowCIDRs) == 0 {
			return errors.New("ADMIN_ALLOW_CIDRS is required when ADMIN_AUTH_ENABLED=true in production-like environments")
		}
	}
	if c.ChaosEnabled {
		if c.ChaosErrorRate < 0 || c.ChaosErrorRate > 1 {
			return fmt.Errorf("CHAOS_ERROR_RATE must be between 0.0 and 1.0, got %f", c.ChaosErrorRate)
		}
		if c.ChaosLatencyRate < 0 || c.ChaosLatencyRate > 1 {
			return fmt.Errorf("CHAOS_LATENCY_RATE must be between 0.0 and 1.0, got %f", c.ChaosLatencyRate)
		}
		if c.ChaosLatencyMinMs < 0 {
			return fmt.Errorf("CHAOS_LATENCY_MIN_MS must be >= 0, got %d", c.ChaosLatencyMinMs)
		}
		if c.ChaosLatencyMaxMs < 0 || c.ChaosLatencyMaxMs > 30000 {
			return fmt.Errorf("CHAOS_LATENCY_MAX_MS must be between 0 and 30000, got %d", c.ChaosLatencyMaxMs)
		}
		if c.ChaosLatencyMinMs > c.ChaosLatencyMaxMs {
			return fmt.Errorf(
				"CHAOS_LATENCY_MIN_MS (%d) must be <= CHAOS_LATENCY_MAX_MS (%d)",
				c.ChaosLatencyMinMs,
				c.ChaosLatencyMaxMs,
			)
		}
	}
	if c.SourceRateLimitEnabled {
		if c.SourceRateLimitDefault <= 0 || c.SourceRateLimitDefault > 1000000 {
			return fmt.Errorf("SOURCE_RATE_LIMIT_DEFAULT must be between 1 and 1000000, got %d", c.SourceRateLimitDefault)
		}
		for src, limit := range c.SourceRateLimits {
			if limit <= 0 || limit > 1000000 {
				return fmt.Errorf("source rate limit for %q must be between 1 and 1000000, got %d", src, limit)
			}
		}
	}
	if !c.RequireSecrets {
		if !IsNonProdEnvironment(c.Environment) {
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
	if err != nil {
		slog.Warn("invalid integer config, using default", "key", key, "value", v, "default", fallback)
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

func parseSourceRateLimits(raw string) map[string]int {
	if raw == "" {
		return nil
	}

	result := make(map[string]int)
	pairs := strings.Split(raw, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		source := strings.TrimSpace(parts[0])
		limit, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || limit <= 0 {
			continue
		}
		result[source] = limit
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func IsNonProdEnvironment(env string) bool {
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
		"change-me":  {},
		"changeme":   {},
		"secret":     {},
		"password":   {},
		"admin":      {},
		"test":       {},
		"default":    {},
		"12345678":   {},
		"123456789":  {},
		"1234567890": {},
		"qwerty":     {},
		"abc123":     {},
		"password1":  {},
		"letmein":    {},
		"welcome":    {},
		"monkey":     {},
		"master":     {},
		"dragon":     {},
	}
	if _, ok := weakValues[v]; ok {
		return true
	}
	if v != "" && strings.Count(v, string(v[0])) == len(v) {
		return true
	}
	if len(v) >= 3 {
		allSequential := true
		for i := 1; i < len(v); i++ {
			if v[i] != v[i-1]+1 {
				allSequential = false
				break
			}
		}
		if allSequential {
			return true
		}
		allReverseSequential := true
		for i := 1; i < len(v); i++ {
			if v[i] != v[i-1]-1 {
				allReverseSequential = false
				break
			}
		}
		if allReverseSequential {
			return true
		}
	}
	return false
}
