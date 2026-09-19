// Package main runs the Pub/Sub-to-Postgres webhook event-store consumer.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"github.com/jackc/pgx/v5/pgxpool"

	"teampulsebridge/services/ingestion-gateway/internal/eventstore"
)

type runtimeConfig struct {
	DatabaseURL            string
	PubSubProjectID        string
	PubSubSubscriptionID   string
	MaxOutstandingMessages int
	MaxOutstandingBytes    int
	ReceiveGoroutines      int
	MaxExtension           time.Duration
	DBMaxConns             int32
	DBMinConns             int32
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("event store exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := loadConfig()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse postgres config: %w", err)
	}
	// Tuned pool: default 4 conns starved under burst (pool starvation =>
	// PubSub lease expiry => duplicate Nack storm).
	if cfg.DBMaxConns > 0 {
		poolCfg.MaxConns = cfg.DBMaxConns
	} else {
		poolCfg.MaxConns = 32
	}
	if cfg.DBMinConns > 0 {
		poolCfg.MinConns = cfg.DBMinConns
	} else {
		poolCfg.MinConns = 8
	}
	poolCfg.MaxConnLifetime = 5 * time.Minute
	poolCfg.MaxConnIdleTime = time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second
	poolCfg.ConnConfig.RuntimeParams["statement_cache_mode"] = "prepare"
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create postgres pool: %w", err)
	}
	defer pool.Close()

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	store, err := eventstore.NewPostgresStore(initCtx, pool)
	cancel()
	if err != nil {
		return fmt.Errorf("initialize event store: %w", err)
	}

	client, err := newPubSubClientWithRetry(ctx, cfg.PubSubProjectID, logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			logger.Warn("close pubsub client", "error", err)
		}
	}()

	subscriptionName := fmt.Sprintf("projects/%s/subscriptions/%s", cfg.PubSubProjectID, cfg.PubSubSubscriptionID)
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if _, err := client.SubscriptionAdminClient.GetSubscription(checkCtx, &pubsubpb.GetSubscriptionRequest{Subscription: subscriptionName}); err != nil {
		cancel()
		return fmt.Errorf("check pubsub subscription %q: %w", cfg.PubSubSubscriptionID, err)
	}
	cancel()

	subscriber := client.Subscriber(cfg.PubSubSubscriptionID)
	subscriber.ReceiveSettings.MaxOutstandingMessages = cfg.MaxOutstandingMessages
	subscriber.ReceiveSettings.MaxOutstandingBytes = cfg.MaxOutstandingBytes
	subscriber.ReceiveSettings.NumGoroutines = cfg.ReceiveGoroutines
	if cfg.MaxExtension > 0 {
		subscriber.ReceiveSettings.MaxExtension = cfg.MaxExtension
	} else {
		subscriber.ReceiveSettings.MaxExtension = 10 * time.Minute
	}

	consumer := eventstore.NewConsumer(subscriber, store, logger)
	logger.Info("starting event store consumer",
		"project_id", cfg.PubSubProjectID,
		"subscription_id", cfg.PubSubSubscriptionID,
		"max_outstanding_messages", cfg.MaxOutstandingMessages,
		"max_outstanding_bytes", cfg.MaxOutstandingBytes,
		"receive_goroutines", cfg.ReceiveGoroutines,
		"max_extension", subscriber.ReceiveSettings.MaxExtension.String(),
		"db_max_conns", poolCfg.MaxConns,
		"db_min_conns", poolCfg.MinConns,
	)
	if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("event store consumer stopped")
	return nil
}

func newPubSubClientWithRetry(ctx context.Context, projectID string, logger *slog.Logger) (*pubsub.Client, error) {
	var client *pubsub.Client
	var err error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= 5; attempt++ {
		client, err = pubsub.NewClient(ctx, projectID)
		if err == nil {
			return client, nil
		}
		logger.Warn("pubsub client init failed, retrying", "attempt", attempt, "error", err, "backoff_ms", backoff.Milliseconds())
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
			if backoff > 8*time.Second {
				backoff = 8 * time.Second
			}
		}
	}
	return nil, fmt.Errorf("create pubsub client after retries: %w", err)
}

func loadConfig() runtimeConfig {
	return runtimeConfig{
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		PubSubProjectID:        os.Getenv("PUBSUB_PROJECT_ID"),
		PubSubSubscriptionID:   envOrDefault("EVENTSTORE_PUBSUB_SUBSCRIPTION_ID", "webhook-events-store"),
		MaxOutstandingMessages: intOrDefault("EVENTSTORE_MAX_OUTSTANDING_MESSAGES", 2000),
		MaxOutstandingBytes:    intOrDefault("EVENTSTORE_MAX_OUTSTANDING_BYTES", 256<<20),
		ReceiveGoroutines:      intOrDefault("EVENTSTORE_RECEIVE_GOROUTINES", 16),
		MaxExtension:           time.Duration(intOrDefault("EVENTSTORE_MAX_EXTENSION_SEC", 600)) * time.Second,
		DBMaxConns:             int32(intOrDefault("EVENTSTORE_DB_MAX_CONNS", 32)),
		DBMinConns:             int32(intOrDefault("EVENTSTORE_DB_MIN_CONNS", 8)),
	}
}

func (c runtimeConfig) Validate() error {
	missing := make([]string, 0, 3)
	if strings.TrimSpace(c.DatabaseURL) == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if strings.TrimSpace(c.PubSubProjectID) == "" {
		missing = append(missing, "PUBSUB_PROJECT_ID")
	}
	if strings.TrimSpace(c.PubSubSubscriptionID) == "" {
		missing = append(missing, "EVENTSTORE_PUBSUB_SUBSCRIPTION_ID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required values: %s", strings.Join(missing, ", "))
	}
	if strings.ContainsAny(c.PubSubProjectID, " \t\n\r") {
		return errors.New("PUBSUB_PROJECT_ID must not contain whitespace")
	}
	if strings.ContainsAny(c.PubSubSubscriptionID, " \t\n\r") {
		return errors.New("EVENTSTORE_PUBSUB_SUBSCRIPTION_ID must not contain whitespace")
	}
	if c.MaxOutstandingMessages < 1 || c.MaxOutstandingMessages > 100000 {
		return fmt.Errorf("EVENTSTORE_MAX_OUTSTANDING_MESSAGES must be between 1 and 100000, got %d", c.MaxOutstandingMessages)
	}
	if c.MaxOutstandingBytes < 0 || c.MaxOutstandingBytes > 4<<30 {
		return fmt.Errorf("EVENTSTORE_MAX_OUTSTANDING_BYTES must be between 0 and 4GiB, got %d", c.MaxOutstandingBytes)
	}
	if c.ReceiveGoroutines < 1 || c.ReceiveGoroutines > 128 {
		return fmt.Errorf("EVENTSTORE_RECEIVE_GOROUTINES must be between 1 and 128, got %d", c.ReceiveGoroutines)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func intOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
