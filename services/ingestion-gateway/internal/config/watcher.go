package config

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	current  atomic.Pointer[Config]
	watcher  *fsnotify.Watcher
	logger   *slog.Logger
	onChange func(Config)
	stopCh   chan struct{}
	stopped  atomic.Bool
}

func NewWatcher(cfg Config, logger *slog.Logger, onChange func(Config)) (*Watcher, error) {
	w := &Watcher{
		logger:   logger,
		onChange: onChange,
		stopCh:   make(chan struct{}),
	}
	w.current.Store(&cfg)

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create file watcher: %w", err)
	}
	w.watcher = fsWatcher

	return w, nil
}

func (w *Watcher) Start(path string) error {
	if path == "" {
		return errors.New("config path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("config file not found: %w", err)
	}

	if err := w.watcher.Add(filepath.Dir(absPath)); err != nil {
		return fmt.Errorf("watch config directory: %w", err)
	}

	go w.watchLoop(absPath)

	w.logger.Info("config watcher started", "path", absPath)
	return nil
}

func (w *Watcher) watchLoop(targetPath string) {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Name == targetPath && (event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create) {
				w.reload(targetPath)
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("config watcher error", "error", err)
		case <-w.stopCh:
			return
		}
	}
}

func (w *Watcher) reload(path string) {
	newCfg, err := LoadFromFile(path)
	if err != nil {
		w.logger.Error("config reload failed", "path", path, "error", err)
		return
	}

	if err := newCfg.Validate(); err != nil {
		w.logger.Error("reloaded config validation failed — keeping current config", "path", path, "error", err)
		return
	}

	current := w.Get()
	if configsEqual(current, newCfg) {
		w.logger.Debug("config unchanged, skipping reload", "path", path)
		return
	}

	w.current.Store(&newCfg)
	w.logger.Info("config reloaded successfully", "path", path)

	if w.onChange != nil {
		w.onChange(newCfg)
	}
}

func configsEqual(a, b Config) bool {
	return a.Environment == b.Environment &&
		a.Port == b.Port &&
		a.QueueBackend == b.QueueBackend &&
		a.QueueBuffer == b.QueueBuffer &&
		a.QueueWorkers == b.QueueWorkers &&
		a.QueueBulkheadEnabled == b.QueueBulkheadEnabled &&
		a.QueueBulkheadBufferPerSource == b.QueueBulkheadBufferPerSource &&
		a.QueueBackpressureEnabled == b.QueueBackpressureEnabled &&
		a.QueueBackpressureSoftLimitPercent == b.QueueBackpressureSoftLimitPercent &&
		a.QueueBackpressureHardLimitPercent == b.QueueBackpressureHardLimitPercent &&
		a.QueueFailureBudgetPercent == b.QueueFailureBudgetPercent &&
		a.QueueFailureBudgetWindow == b.QueueFailureBudgetWindow &&
		a.QueueFailureBudgetMinSamples == b.QueueFailureBudgetMinSamples &&
		a.QueueThrottleRetryAfterSec == b.QueueThrottleRetryAfterSec &&
		a.RequestTimeoutSec == b.RequestTimeoutSec &&
		a.RequireSecrets == b.RequireSecrets &&
		a.RateLimitEnabled == b.RateLimitEnabled &&
		a.RateLimitRPM == b.RateLimitRPM &&
		a.AdminRateLimitRPM == b.AdminRateLimitRPM &&
		a.RateLimitBackend == b.RateLimitBackend &&
		a.RateLimitRedisPrefix == b.RateLimitRedisPrefix &&
		a.DedupEnabled == b.DedupEnabled &&
		a.DedupTTLSeconds == b.DedupTTLSeconds &&
		a.DedupRedisPrefix == b.DedupRedisPrefix &&
		a.SchemaValidationEnabled == b.SchemaValidationEnabled &&
		a.SchemaPath == b.SchemaPath &&
		a.RetryEnabled == b.RetryEnabled &&
		a.RetryMaxAttempts == b.RetryMaxAttempts &&
		a.RetryIntervalSec == b.RetryIntervalSec &&
		a.SourceRateLimitEnabled == b.SourceRateLimitEnabled &&
		a.SourceRateLimitDefault == b.SourceRateLimitDefault &&
		a.PIIScrubbingEnabled == b.PIIScrubbingEnabled &&
		a.AdminAuthEnabled == b.AdminAuthEnabled &&
		a.FailedStoreEnabled == b.FailedStoreEnabled &&
		a.FailedStorePath == b.FailedStorePath &&
		a.ReplayAuditEnabled == b.ReplayAuditEnabled &&
		a.ReplayAuditPath == b.ReplayAuditPath &&
		a.SecurityAuditEnabled == b.SecurityAuditEnabled &&
		a.SecurityAuditPath == b.SecurityAuditPath &&
		a.SecurityAuditRetentionDays == b.SecurityAuditRetentionDays &&
		a.RedisAddr == b.RedisAddr &&
		a.RedisPassword == b.RedisPassword &&
		a.RedisDB == b.RedisDB &&
		a.DatabaseURL == b.DatabaseURL &&
		a.PubSubProjectID == b.PubSubProjectID &&
		a.PubSubTopicID == b.PubSubTopicID &&
		a.PubSubPublishTimeoutSec == b.PubSubPublishTimeoutSec &&
		a.PubSubPublishGoroutines == b.PubSubPublishGoroutines &&
		a.PubSubMaxOutstandingMessages == b.PubSubMaxOutstandingMessages &&
		a.PubSubMaxOutstandingBytes == b.PubSubMaxOutstandingBytes &&
		a.PubSubFlowControlBehavior == b.PubSubFlowControlBehavior &&
		a.AdminJWTSecret == b.AdminJWTSecret &&
		a.AdminJWTIssuer == b.AdminJWTIssuer &&
		a.AdminJWTAudience == b.AdminJWTAudience &&
		a.SlackSigningSecret == b.SlackSigningSecret &&
		a.GitHubWebhookSecret == b.GitHubWebhookSecret &&
		a.GitLabWebhookToken == b.GitLabWebhookToken &&
		a.TeamsClientState == b.TeamsClientState &&
		a.ChaosEnabled == b.ChaosEnabled &&
		a.ChaosErrorRate == b.ChaosErrorRate &&
		a.ChaosLatencyRate == b.ChaosLatencyRate &&
		a.ChaosLatencyMinMs == b.ChaosLatencyMinMs &&
		a.ChaosLatencyMaxMs == b.ChaosLatencyMaxMs &&
		slices.Equal(a.AdminAllowCIDRs, b.AdminAllowCIDRs) &&
		slices.Equal(a.TrustedProxyCIDRs, b.TrustedProxyCIDRs) &&
		maps.Equal(a.SourceRateLimits, b.SourceRateLimits)
}

func (w *Watcher) Get() Config {
	ptr := w.current.Load()
	if ptr == nil {
		return Config{}
	}
	return *ptr
}

func (w *Watcher) Stop() error {
	if w.stopped.Swap(true) {
		return nil
	}
	close(w.stopCh)
	if err := w.watcher.Close(); err != nil {
		return fmt.Errorf("close file watcher: %w", err)
	}
	return nil
}

func LoadFromFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	cfg, err := ParseConfig(data)
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func ParseConfig(data []byte) (Config, error) {
	envOverrides := make(map[string]string)

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		envOverrides[key] = value
	}

	return LoadFromMap(envOverrides), nil
}

func LoadFromMap(overrides map[string]string) Config {
	get := func(key, fallback string) string {
		if v, ok := overrides[key]; ok && v != "" {
			return v
		}
		return fallback
	}
	getEnv := func(key string) string {
		if v, ok := overrides[key]; ok {
			return v
		}
		return os.Getenv(key)
	}
	getInt := func(key string, fallback int) int {
		if v, ok := overrides[key]; ok && v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
		return fallback
	}
	getBool := func(key string, fallback bool) bool {
		if v, ok := overrides[key]; ok && v != "" {
			v = strings.ToLower(strings.TrimSpace(v))
			switch v {
			case "1", "true", "yes", "y":
				return true
			case "0", "false", "no", "n":
				return false
			}
		}
		return fallback
	}
	getFloat := func(key string, fallback float64) float64 {
		if v, ok := overrides[key]; ok && v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		}
		return fallback
	}
	getCSV := func(key string) []string {
		if v, ok := overrides[key]; ok {
			v = strings.TrimSpace(v)
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
			if len(out) > 0 {
				return out
			}
		}
		return splitCSVEnv(key)
	}

	return Config{
		Environment:                       get("ENVIRONMENT", "dev"),
		Port:                              get("PORT", "8080"),
		SlackSigningSecret:                getEnv("SLACK_SIGNING_SECRET"),
		GitHubWebhookSecret:               getEnv("GITHUB_WEBHOOK_SECRET"),
		GitLabWebhookToken:                getEnv("GITLAB_WEBHOOK_TOKEN"),
		TeamsClientState:                  getEnv("TEAMS_CLIENT_STATE"),
		QueueBuffer:                       getInt("QUEUE_BUFFER", 4096),
		QueueWorkers:                      getInt("QUEUE_WORKERS", 1),
		QueueBulkheadEnabled:              getBool("QUEUE_BULKHEAD_ENABLED", false),
		QueueBulkheadBufferPerSource:      getInt("QUEUE_BULKHEAD_BUFFER_PER_SOURCE", 1024),
		RequestTimeoutSec:                 getInt("REQUEST_TIMEOUT_SEC", 15),
		RequireSecrets:                    getBool("REQUIRE_SECRETS", true),
		QueueBackend:                      get("QUEUE_BACKEND", "log"),
		PubSubProjectID:                   getEnv("PUBSUB_PROJECT_ID"),
		PubSubTopicID:                     getEnv("PUBSUB_TOPIC_ID"),
		AdminAuthEnabled:                  getBool("ADMIN_AUTH_ENABLED", true),
		AdminJWTIssuer:                    getEnv("ADMIN_JWT_ISSUER"),
		AdminJWTAudience:                  getEnv("ADMIN_JWT_AUDIENCE"),
		AdminJWTSecret:                    getEnv("ADMIN_JWT_SECRET"),
		AdminAllowCIDRs:                   getCSV("ADMIN_ALLOW_CIDRS"),
		TrustedProxyCIDRs:                 getCSV("TRUSTED_PROXY_CIDRS"),
		RateLimitEnabled:                  getBool("RATE_LIMIT_ENABLED", true),
		RateLimitRPM:                      getInt("RATE_LIMIT_RPM", 300),
		AdminRateLimitRPM:                 getInt("ADMIN_RATE_LIMIT_RPM", 60),
		DedupEnabled:                      getBool("DEDUP_ENABLED", true),
		DedupTTLSeconds:                   getInt("DEDUP_TTL_SEC", 300),
		RedisAddr:                         getEnv("REDIS_ADDR"),
		RedisPassword:                     getEnv("REDIS_PASSWORD"),
		RedisDB:                           getInt("REDIS_DB", 0),
		DedupRedisPrefix:                  get("DEDUP_REDIS_PREFIX", "webhook_dedup"),
		FailedStoreEnabled:                getBool("FAILED_EVENT_STORE_ENABLED", true),
		FailedStorePath:                   get("FAILED_EVENT_STORE_PATH", "data/failed-events.jsonl"),
		DatabaseURL:                       getEnv("DATABASE_URL"),
		ReplayAuditEnabled:                getBool("REPLAY_AUDIT_ENABLED", true),
		ReplayAuditPath:                   get("REPLAY_AUDIT_PATH", "data/replay-audit.jsonl"),
		SecurityAuditEnabled:              getBool("SECURITY_AUDIT_ENABLED", true),
		SecurityAuditPath:                 get("SECURITY_AUDIT_PATH", "data/security-audit.jsonl"),
		SecurityAuditRetentionDays:        getInt("SECURITY_AUDIT_RETENTION_DAYS", 30),
		QueueBackpressureEnabled:          getBool("QUEUE_BACKPRESSURE_ENABLED", true),
		QueueBackpressureSoftLimitPercent: getInt("QUEUE_BACKPRESSURE_SOFT_LIMIT_PERCENT", 70),
		QueueBackpressureHardLimitPercent: getInt("QUEUE_BACKPRESSURE_HARD_LIMIT_PERCENT", 90),
		QueueFailureBudgetPercent:         getInt("QUEUE_FAILURE_BUDGET_PERCENT", 15),
		QueueFailureBudgetWindow:          getInt("QUEUE_FAILURE_BUDGET_WINDOW", 100),
		QueueFailureBudgetMinSamples:      getInt("QUEUE_FAILURE_BUDGET_MIN_SAMPLES", 20),
		QueueThrottleRetryAfterSec:        getInt("QUEUE_THROTTLE_RETRY_AFTER_SEC", 5),
		SourceRateLimitEnabled:            getBool("SOURCE_RATE_LIMIT_ENABLED", true),
		SourceRateLimits:                  parseSourceRateLimits(getEnv("SOURCE_RATE_LIMITS")),
		SourceRateLimitDefault:            getInt("SOURCE_RATE_LIMIT_DEFAULT", 100),
		SchemaValidationEnabled:           getBool("SCHEMA_VALIDATION_ENABLED", true),
		SchemaPath:                        get("SCHEMA_PATH", "internal/schema/schemas"),
		RetryEnabled:                      getBool("RETRY_ENABLED", false),
		RetryMaxAttempts:                  getInt("RETRY_MAX_ATTEMPTS", 3),
		RetryIntervalSec:                  getInt("RETRY_INTERVAL_SEC", 10),
		PubSubPublishTimeoutSec:           getInt("PUBSUB_PUBLISH_TIMEOUT_SEC", 5),
		PubSubPublishGoroutines:           getInt("PUBSUB_PUBLISH_GOROUTINES", 0),
		PubSubMaxOutstandingMessages:      getInt("PUBSUB_MAX_OUTSTANDING_MESSAGES", 0),
		PubSubMaxOutstandingBytes:         getInt("PUBSUB_MAX_OUTSTANDING_BYTES", 0),
		PubSubFlowControlBehavior:         get("PUBSUB_FLOW_CONTROL_BEHAVIOR", "ignore"),
		RateLimitBackend:                  get("RATE_LIMIT_BACKEND", "memory"),
		RateLimitRedisPrefix:              get("RATE_LIMIT_REDIS_PREFIX", "rate_limit"),
		PIIScrubbingEnabled:               getBool("PII_SCRUBBING_ENABLED", false),
		ChaosEnabled:                      getBool("CHAOS_ENABLED", false),
		ChaosErrorRate:                    getFloat("CHAOS_ERROR_RATE", 0.0),
		ChaosLatencyRate:                  getFloat("CHAOS_LATENCY_RATE", 0.0),
		ChaosLatencyMinMs:                 getInt("CHAOS_LATENCY_MIN_MS", 100),
		ChaosLatencyMaxMs:                 getInt("CHAOS_LATENCY_MAX_MS", 500),
	}
}
