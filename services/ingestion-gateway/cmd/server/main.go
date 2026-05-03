package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"teampulsebridge/services/ingestion-gateway/internal/config"
	"teampulsebridge/services/ingestion-gateway/internal/dedup"
	"teampulsebridge/services/ingestion-gateway/internal/failstore"
	"teampulsebridge/services/ingestion-gateway/internal/handlers"
	"teampulsebridge/services/ingestion-gateway/internal/httpx"
	"teampulsebridge/services/ingestion-gateway/internal/observability"
	"teampulsebridge/services/ingestion-gateway/internal/queue"
	"teampulsebridge/services/ingestion-gateway/internal/replayaudit"
	"teampulsebridge/services/ingestion-gateway/internal/securityaudit"
)

func newPublicMux(appHandler http.Handler, smokeHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	if smokeHandler != nil {
		mux.Handle("/ui/smoke-test", smokeHandler)
	}
	mux.Handle("/", appHandler)
	return mux
}

type HandlerBuilder struct {
	logger     *slog.Logger
	cfg       *config.Config
	securityFn func(req *http.Request, reason string, status int)
	limiter   *httpx.IPRateLimiter
}

func NewHandlerBuilder(logger *slog.Logger, cfg *config.Config, securityFn func(req *http.Request, reason string, status int)) *HandlerBuilder {
	limiter := httpx.NewIPRateLimiter(nil, time.Minute, 1024)
	return &HandlerBuilder{
		logger:     logger,
		cfg:       cfg,
		securityFn: securityFn,
		limiter:   limiter,
	}
}

func (b *HandlerBuilder) Build(publicMux http.Handler) http.Handler {
	return httpx.Chain(
		publicMux,
		httpx.RequestID(),
		httpx.RateLimit(httpx.RateLimitConfig{
			Enabled:           b.cfg.RateLimitEnabled,
			General:           b.cfg.RateLimitRPM,
			Admin:             b.cfg.AdminRateLimitRPM,
			TrustedProxyCIDRs: b.cfg.TrustedProxyCIDRs,
			OnReject:          b.securityFn,
			Limiter:          b.limiter,
		}),
		httpx.RequireAdminCIDRAllowlist(httpx.AdminCIDRConfig{
			Enabled:           b.cfg.AdminAuthEnabled,
			CIDRs:             b.cfg.AdminAllowCIDRs,
			TrustedProxyCIDRs: b.cfg.TrustedProxyCIDRs,
			OnReject:          b.securityFn,
		}),
		httpx.RequireAdminJWT(httpx.JWTConfig{
			Enabled:  b.cfg.AdminAuthEnabled,
			Issuer:   b.cfg.AdminJWTIssuer,
			Audience: b.cfg.AdminJWTAudience,
			Secret:   b.cfg.AdminJWTSecret,
			OnReject: b.securityFn,
		}),
		httpx.Recoverer(b.logger),
		httpx.AccessLog(b.logger),
		observability.HTTPMiddleware("ingestion-gateway"),
	)
}

func (b *HandlerBuilder) Stop() {
	b.limiter.Stop()
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

	dedupTTL := time.Duration(cfg.DedupTTLSeconds) * time.Second
	if dedupTTL <= 0 {
		dedupTTL = 5 * time.Minute
	}

	var failedStore failstore.Store
	if cfg.FailedStoreEnabled {
		store, err := failstore.NewFileStore(cfg.FailedStorePath)
		if err != nil {
			logger.Error("failed event store disabled due to invalid configuration",
				"path", cfg.FailedStorePath,
				"error", err,
			)
		} else {
			failedStore = store
		}
	}

	var replayAuditStore replayaudit.Store
	if cfg.ReplayAuditEnabled {
		store, err := replayaudit.NewFileStore(cfg.ReplayAuditPath)
		if err != nil {
			logger.Error("replay audit history disabled due to invalid configuration",
				"path", cfg.ReplayAuditPath,
				"error", err,
			)
		} else {
			replayAuditStore = store
		}
	}

	var securityAuditStore securityaudit.Store
	if cfg.SecurityAuditEnabled {
		store, err := securityaudit.NewFileStore(cfg.SecurityAuditPath, cfg.SecurityAuditRetentionDays)
		if err != nil {
			logger.Error("security audit stream disabled due to invalid configuration",
				"path", cfg.SecurityAuditPath,
				"retention_days", cfg.SecurityAuditRetentionDays,
				"error", err,
			)
		} else {
			securityAuditStore = store
		}
	}

	recordSecurityEvent := func(req *http.Request, in securityaudit.SaveInput) {
		reqCtx := req.Context()
		source := strings.TrimSpace(in.Source)
		if source == "" {
			source = securitySourceFromPath(req.URL.Path)
		}
		if strings.TrimSpace(in.Outcome) == "" {
			in.Outcome = "rejected"
		}
		if strings.TrimSpace(in.Path) == "" {
			in.Path = req.URL.Path
		}
		if strings.TrimSpace(in.RequestID) == "" {
			in.RequestID = httpx.RequestIDFromContext(req.Context())
		}
		if strings.TrimSpace(in.ClientIP) == "" {
			in.ClientIP = httpx.ClientIP(req, cfg.TrustedProxyCIDRs)
		}
		if strings.TrimSpace(in.Source) == "" {
			in.Source = source
		}

		telemetry.SecurityRejectCounter.Add(reqCtx, 1,
			metric.WithAttributes(
				attribute.String("reason", in.Reason),
				attribute.String("path", in.Path),
				attribute.Int("status", in.HTTPStatus),
				attribute.String("source", in.Source),
				attribute.String("category", in.Category),
			),
		)
		logger.Warn("security event recorded",
			"audit_stream", "security",
			"category", in.Category,
			"source", in.Source,
			"reason", in.Reason,
			"path", in.Path,
			"status", in.HTTPStatus,
			"request_id", in.RequestID,
			"client_ip", in.ClientIP,
			"actor", in.Actor,
		)

		if securityAuditStore == nil {
			return
		}
		if _, err := securityAuditStore.Save(reqCtx, in); err != nil {
			logger.Error("failed to persist security audit record",
				"category", in.Category,
				"source", in.Source,
				"reason", in.Reason,
				"path", in.Path,
				"status", in.HTTPStatus,
				"request_id", in.RequestID,
				"error", err,
			)
		}
	}

	h := handlers.NewWebhookHandlerWithDependencies(cfg, runtimePublisher.Publisher, logger, func(reqCtx context.Context, source string, status int) {
		telemetry.WebhookCounter.Add(reqCtx, 1,
			metric.WithAttributes(
				attribute.String("source", source),
				attribute.Int("status", status),
			),
		)
	}, dedup.NewMemory(cfg.DedupEnabled, dedupTTL), failedStore, func(r *http.Request, event handlers.SecurityEvent) {
		record := handlers.SecurityAuditRecord(r, event)
		record.ClientIP = httpx.ClientIP(r, cfg.TrustedProxyCIDRs)
		recordSecurityEvent(r, record)
	})
	securityRejectFn := func(req *http.Request, reason string, status int) {
		recordSecurityEvent(req, securityaudit.SaveInput{
			Category:   "request_rejected",
			Outcome:    "rejected",
			Source:     securitySourceFromPath(req.URL.Path),
			Reason:     reason,
			Path:       req.URL.Path,
			HTTPStatus: status,
		})
	}
	admin := handlers.NewAdminHandlerWithDependencies(cfg, runtimePublisher.Publisher, logger, failedStore, replayAuditStore, securityAuditStore)

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
	webhookMux.HandleFunc("GET /admin/events/security-audit", admin.SecurityAudit)
	webhookMux.HandleFunc("POST /admin/events/replay/batch", admin.ReplayFailedEventsBatch)
	webhookMux.HandleFunc("POST /admin/events/replay", admin.ReplayFailedEvent)
	webhookMux.HandleFunc("POST /webhooks/slack", h.HandleSlack)
	webhookMux.HandleFunc("POST /webhooks/teams", h.HandleTeams)
	webhookMux.HandleFunc("POST /webhooks/github", h.HandleGitHub)
	webhookMux.HandleFunc("POST /webhooks/gitlab", h.HandleGitLab)

	publicMux := newPublicMux(webhookMux, nil)

	handlerBuilder := NewHandlerBuilder(logger, &cfg, securityRejectFn)
	handler := handlerBuilder.Build(publicMux)

	uiSmokeProxy := handlers.NewUISmokeTestProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}), cfg.TrustedProxyCIDRs)
	publicMux = newPublicMux(webhookMux, uiSmokeProxy)
	handler = handlerBuilder.Build(publicMux)

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

	handlerBuilder.Stop()
}

func securitySourceFromPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/admin"):
		return "admin"
	case strings.HasPrefix(path, "/webhooks/"):
		source := strings.TrimPrefix(path, "/webhooks/")
		source = strings.Trim(source, "/")
		if source != "" {
			return source
		}
	}
	return "http"
}
