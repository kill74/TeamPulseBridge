package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config contains runtime values for webhook signature validation.
type Config struct {
	Port                string
	SlackSigningSecret  string
	GitHubWebhookSecret string
	GitLabWebhookToken  string
	TeamsClientState    string
	QueueBuffer         int
	RequestTimeoutSec   int
	RequireSecrets      bool
	QueueBackend        string
	PubSubProjectID     string
	PubSubTopicID       string
	AdminAuthEnabled    bool
	AdminJWTIssuer      string
	AdminJWTAudience    string
	AdminJWTSecret      string
}

func LoadFromEnv() Config {
	return Config{
		Port:                envOrDefault("PORT", "8080"),
		SlackSigningSecret:  os.Getenv("SLACK_SIGNING_SECRET"),
		GitHubWebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		GitLabWebhookToken:  os.Getenv("GITLAB_WEBHOOK_TOKEN"),
		TeamsClientState:    os.Getenv("TEAMS_CLIENT_STATE"),
		QueueBuffer:         intOrDefault("QUEUE_BUFFER", 4096),
		RequestTimeoutSec:   intOrDefault("REQUEST_TIMEOUT_SEC", 15),
		RequireSecrets:      boolOrDefault("REQUIRE_SECRETS", true),
		QueueBackend:        envOrDefault("QUEUE_BACKEND", "log"),
		PubSubProjectID:     os.Getenv("PUBSUB_PROJECT_ID"),
		PubSubTopicID:       os.Getenv("PUBSUB_TOPIC_ID"),
		AdminAuthEnabled:    boolOrDefault("ADMIN_AUTH_ENABLED", false),
		AdminJWTIssuer:      os.Getenv("ADMIN_JWT_ISSUER"),
		AdminJWTAudience:    os.Getenv("ADMIN_JWT_AUDIENCE"),
		AdminJWTSecret:      os.Getenv("ADMIN_JWT_SECRET"),
	}
}

func (c Config) Validate() error {
	if c.Port == "" {
		return errors.New("PORT must not be empty")
	}
	if c.QueueBuffer <= 0 {
		return fmt.Errorf("QUEUE_BUFFER must be > 0, got %d", c.QueueBuffer)
	}
	if c.RequestTimeoutSec <= 0 {
		return fmt.Errorf("REQUEST_TIMEOUT_SEC must be > 0, got %d", c.RequestTimeoutSec)
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
	}
	if !c.RequireSecrets {
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
