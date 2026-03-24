package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/handlers"
	"teampulsebridge/services/ingestion-gateway/internal/httpx"
	"teampulsebridge/services/ingestion-gateway/internal/observability"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

func main() {
	logger := observability.NewLogger()
	cfg := config.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	telemetry, err := observability.Setup(ctx, logger, "ingestion-gateway")
	if err != nil {
		logger.Error("telemetry setup failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := telemetry.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.Error("telemetry shutdown failed", "error", shutdownErr)
		}
	}()

	runtimePublisher, err := queue.BuildRuntimePublisher(ctx, cfg, logger)
	if err != nil {
		logger.Error("publisher initialization failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := runtimePublisher.Close(); closeErr != nil {
			logger.Error("publisher close failed", "error", closeErr)
		}
	}()

	h := handlers.NewWebhookHandler(cfg, runtimePublisher.Publisher, logger, func(reqCtx context.Context, source string, status int) {
		telemetry.WebhookCounter.Add(reqCtx, 1,
			metric.WithAttributes(
				attribute.String("source", source),
				attribute.Int("status", status),
			),
		)
	})
	admin := handlers.NewAdminHandler(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handlers.ProductUI)
	mux.HandleFunc("GET /assets/ui.css", handlers.ProductUIStyles)
	mux.HandleFunc("GET /assets/ui.js", handlers.ProductUIScript)
	mux.HandleFunc("POST /ui/smoke-test", handlers.NewUISmokeTestProxy(mux))
	mux.HandleFunc("GET /healthz", handlers.Healthz)
	mux.HandleFunc("GET /readyz", handlers.Readyz)
	mux.Handle("GET /metrics", telemetry.MetricsHandler)
	mux.HandleFunc("GET /admin/configz", admin.Configz)
	mux.HandleFunc("POST /webhooks/slack", h.HandleSlack)
	mux.HandleFunc("POST /webhooks/teams", h.HandleTeams)
	mux.HandleFunc("POST /webhooks/github", h.HandleGitHub)
	mux.HandleFunc("POST /webhooks/gitlab", h.HandleGitLab)
	handler := httpx.Chain(
		mux,
		httpx.RequestID(),
		httpx.RequireAdminJWT(httpx.JWTConfig{
			Enabled:  cfg.AdminAuthEnabled,
			Issuer:   cfg.AdminJWTIssuer,
			Audience: cfg.AdminJWTAudience,
			Secret:   cfg.AdminJWTSecret,
		}),
		httpx.Recoverer(logger),
		httpx.AccessLog(logger),
		observability.HTTPMiddleware("ingestion-gateway"),
	)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("ingestion-gateway listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}
