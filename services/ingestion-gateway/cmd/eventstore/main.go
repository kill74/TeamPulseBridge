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
	ReceiveGoroutines      int
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

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
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

	client, err := pubsub.NewClient(ctx, cfg.PubSubProjectID)
	if err != nil {
		return fmt.Errorf("create pubsub client: %w", err)
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
	subscriber.ReceiveSettings.NumGoroutines = cfg.ReceiveGoroutines

	consumer := eventstore.NewConsumer(subscriber, store, logger)
	logger.Info("starting event store consumer",
		"project_id", cfg.PubSubProjectID,
		"subscription_id", cfg.PubSubSubscriptionID,
		"max_outstanding_messages", cfg.MaxOutstandingMessages,
		"receive_goroutines", cfg.ReceiveGoroutines,
	)
	if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	logger.Info("event store consumer stopped")
	return nil
}

func loadConfig() runtimeConfig {
	return runtimeConfig{
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		PubSubProjectID:        os.Getenv("PUBSUB_PROJECT_ID"),
		PubSubSubscriptionID:   envOrDefault("EVENTSTORE_PUBSUB_SUBSCRIPTION_ID", "webhook-events-store"),
		MaxOutstandingMessages: intOrDefault("EVENTSTORE_MAX_OUTSTANDING_MESSAGES", 100),
		ReceiveGoroutines:      intOrDefault("EVENTSTORE_RECEIVE_GOROUTINES", 1),
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
