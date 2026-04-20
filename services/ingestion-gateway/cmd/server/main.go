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

func newPublicMux(appHandler http.Handler, smokeHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/ui/smoke-test", smokeHandler)
	mux.Handle("/", appHandler)
	return mux
}

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

	runtimePublisher, err := queue.BuildRuntimePublisher(ctx, cfg, logger, queue.AsyncPublisherOptions{
		Hooks: queue.AsyncPublisherHooks{
			OnBackpressure: func(metricCtx context.Context, source, action string, _ queue.PublisherSnapshot) {
				telemetry.QueueBackpressureCounter.Add(metricCtx, 1,
					metric.WithAttributes(
						attribute.String("source", source),
						attribute.String("action", action),
					),
				)
			},
			OnPublish: func(metricCtx context.Context, source, result string, _ queue.PublisherSnapshot) {
				telemetry.QueuePublishCounter.Add(metricCtx, 1,
					metric.WithAttributes(
						attribute.String("source", source),
						attribute.String("result", result),
					),
				)
			},
		},
	})
	if err != nil {
		logger.Error("publisher initialization failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := runtimePublisher.Close(); closeErr != nil {
			logger.Error("publisher close failed", "error", closeErr)
		}
	}()
	if err := telemetry.BindQueueMetrics("ingestion-gateway", runtimePublisher); err != nil {
		logger.Error("queue telemetry binding failed", "error", err)
		os.Exit(1)
	}

	h := handlers.NewWebhookHandler(cfg, runtimePublisher.Publisher, logger, func(reqCtx context.Context, source string, status int) {
		telemetry.WebhookCounter.Add(reqCtx, 1,
			metric.WithAttributes(
				attribute.String("source", source),
				attribute.Int("status", status),
			),
		)
	})
	securityRejectFn := func(req *http.Request, reason string, status int) {
		telemetry.SecurityRejectCounter.Add(req.Context(), 1,
			metric.WithAttributes(
				attribute.String("reason", reason),
				attribute.String("path", req.URL.Path),
				attribute.Int("status", status),
			),
		)
	}
	admin := handlers.NewAdminHandler(cfg, runtimePublisher.Publisher, logger)

	webhookMux := http.NewServeMux()
	webhookMux.HandleFunc("GET /", handlers.ProductUI)
	webhookMux.HandleFunc("GET /assets/ui.css", handlers.ProductUIStyles)
	webhookMux.HandleFunc("GET /assets/ui.js", handlers.ProductUIScript)
	webhookMux.HandleFunc("GET /healthz", handlers.Healthz)
	webhookMux.HandleFunc("GET /readyz", handlers.Readyz)
	webhookMux.Handle("GET /metrics", telemetry.MetricsHandler)
	webhookMux.HandleFunc("GET /admin/configz", admin.Configz)
	webhookMux.HandleFunc("GET /admin/events/failed", admin.FailedEvents)
	webhookMux.HandleFunc("GET /admin/events/replay-audit", admin.ReplayAudit)
	webhookMux.HandleFunc("POST /admin/events/replay/batch", admin.ReplayFailedEventsBatch)
	webhookMux.HandleFunc("POST /admin/events/replay", admin.ReplayFailedEvent)
	webhookMux.HandleFunc("POST /webhooks/slack", h.HandleSlack)
	webhookMux.HandleFunc("POST /webhooks/teams", h.HandleTeams)
	webhookMux.HandleFunc("POST /webhooks/github", h.HandleGitHub)
	webhookMux.HandleFunc("POST /webhooks/gitlab", h.HandleGitLab)

	var handler http.Handler
	publicMux := newPublicMux(
		webhookMux,
		handlers.NewUISmokeTestProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(w, r)
		}), cfg.TrustedProxyCIDRs),
	)

	handler = httpx.Chain(
		publicMux,
		httpx.RequestID(),
		httpx.RateLimit(httpx.RateLimitConfig{
			Enabled:           cfg.RateLimitEnabled,
			General:           cfg.RateLimitRPM,
			Admin:             cfg.AdminRateLimitRPM,
			TrustedProxyCIDRs: cfg.TrustedProxyCIDRs,
			OnReject:          securityRejectFn,
		}),
		httpx.RequireAdminCIDRAllowlist(httpx.AdminCIDRConfig{
			Enabled:           cfg.AdminAuthEnabled,
			CIDRs:             cfg.AdminAllowCIDRs,
			TrustedProxyCIDRs: cfg.TrustedProxyCIDRs,
			OnReject:          securityRejectFn,
		}),
		httpx.RequireAdminJWT(httpx.JWTConfig{
			Enabled:  cfg.AdminAuthEnabled,
			Issuer:   cfg.AdminJWTIssuer,
			Audience: cfg.AdminJWTAudience,
			Secret:   cfg.AdminJWTSecret,
			OnReject: securityRejectFn,
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
