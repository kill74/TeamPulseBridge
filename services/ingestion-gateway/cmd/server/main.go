package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
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
	"teampulsebridge/services/ingestion-gateway/internal/retry"
	"teampulsebridge/services/ingestion-gateway/internal/schema"
	"teampulsebridge/services/ingestion-gateway/internal/securityaudit"
)

var buildVersion = "dev"

func newPublicMux(appHandler http.Handler, smokeHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	if smokeHandler != nil {
		mux.Handle("/ui/smoke-test", smokeHandler)
	}
	mux.Handle("/", appHandler)
	return mux
}

type deferredHandler struct {
	mu      sync.RWMutex
	handler http.Handler
}

func (d *deferredHandler) set(h http.Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handler = h
}

func (d *deferredHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.handler != nil {
		d.handler.ServeHTTP(w, r)
	}
}

type HandlerBuilder struct {
	logger      *slog.Logger
	cfg         *config.Config
	securityFn  func(req *http.Request, reason string, status int)
	limiter     httpx.RateLimiter
	stopLimiter func()
}

func NewHandlerBuilder(logger *slog.Logger, cfg *config.Config, securityFn func(req *http.Request, reason string, status int)) *HandlerBuilder {
	limiter := httpx.NewIPRateLimiter(nil, time.Minute, 1024)
	return NewHandlerBuilderWithLimiter(logger, cfg, securityFn, limiter, limiter.Stop)
}

func NewHandlerBuilderWithLimiter(
	logger *slog.Logger,
	cfg *config.Config,
	securityFn func(req *http.Request, reason string, status int),
	limiter httpx.RateLimiter,
	stopLimiter func(),
) *HandlerBuilder {
	if limiter == nil {
		memoryLimiter := httpx.NewIPRateLimiter(nil, time.Minute, 1024)
		limiter = memoryLimiter
		stopLimiter = memoryLimiter.Stop
	}
	return &HandlerBuilder{
		logger:      logger,
		cfg:         cfg,
		securityFn:  securityFn,
		limiter:     limiter,
		stopLimiter: stopLimiter,
	}
}

func (b *HandlerBuilder) Build(publicMux http.Handler, durationHistogram metric.Float64Histogram) http.Handler {
	timeoutSec := time.Duration(b.cfg.RequestTimeoutSec) * time.Second
	return httpx.Chain(
		publicMux,
		httpx.APIVersionMiddleware(httpx.APIVersionConfig{
			Enabled:    true,
			Version:    httpx.CurrentAPIVersion,
			SunsetDate: httpx.DefaultSunsetDate,
			OnVersionMismatch: func(w http.ResponseWriter, r *http.Request, requested string) {
				http.Error(w, fmt.Sprintf("unsupported API version: %s (current: %s)", requested, httpx.CurrentAPIVersion), http.StatusNotAcceptable)
			},
		}),
		httpx.RequestTimeout(timeoutSec),
		httpx.RequestID(),
		httpx.RateLimit(httpx.RateLimitConfig{
			Enabled:           b.cfg.RateLimitEnabled,
			General:           b.cfg.RateLimitRPM,
			Admin:             b.cfg.AdminRateLimitRPM,
			TrustedProxyCIDRs: b.cfg.TrustedProxyCIDRs,
			OnReject:          b.securityFn,
			Limiter:           b.limiter,
		}),
		httpx.SourceRateLimit(httpx.SourceRateLimitConfig{
			Enabled:           b.cfg.SourceRateLimitEnabled,
			Sources:           b.cfg.SourceRateLimits,
			Default:           b.cfg.SourceRateLimitDefault,
			TrustedProxyCIDRs: b.cfg.TrustedProxyCIDRs,
			Limiter:           b.limiter,
			OnReject: func(r *http.Request, source string, status int) {
				b.securityFn(r, "source_rate_limit_exceeded:"+source, status)
			},
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
		httpx.AccessLog(b.logger, durationHistogram),
		observability.HTTPMiddleware("ingestion-gateway"),
	)
}

func (b *HandlerBuilder) Stop() {
	if b.stopLimiter != nil {
		b.stopLimiter()
	}
}

func main() {
	cfg := config.LoadFromEnv()
	if err := cfg.Validate(); err != nil {
		logger := observability.NewLogger("ingestion-gateway", "unknown", buildVersion)
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	logger := observability.NewLogger("ingestion-gateway", cfg.Environment, buildVersion)

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

	var failedStore failstore.Store
	var deduper dedup.Store

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
			OnDLQ: func(dlqCtx context.Context, source string, body []byte, headers map[string]string, err error) {
				eventID := strings.TrimSpace(headers["X-Event-ID"])
				if deduper != nil && eventID != "" {
					deduper.Forget(source + ":" + eventID)
				}
				logger.Error("event sent to dead letter queue",
					"source", source,
					"event_id", eventID,
					"error", err,
				)
				if failedStore == nil {
					return
				}
				persistCtx, cancel := context.WithTimeout(dlqCtx, 5*time.Second)
				defer cancel()
				if _, saveErr := failedStore.Save(persistCtx, failstore.SaveInput{
					EventID: eventID,
					Source:  source,
					Reason:  "async_publish_failed",
					Headers: headers,
					Body:    body,
				}); saveErr != nil {
					logger.Error("failed to persist async publish failure",
						"source", source,
						"event_id", eventID,
						"error", saveErr,
					)
				}
			},
			OnPublishLatency: func(latencyCtx context.Context, source string, latencySec float64, failed bool) {
				if telemetry.QueuePublishLatency != nil {
					telemetry.QueuePublishLatency.Record(latencyCtx, latencySec,
						metric.WithAttributes(
							attribute.String("source", source),
							attribute.Bool("failed", failed),
						),
					)
				}
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

	var redisClient *redis.Client
	if cfg.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})
		deduper = dedup.NewRedis(cfg.DedupEnabled, redisClient, cfg.DedupRedisPrefix, dedupTTL, logger)
		logger.Info("using redis for deduplication", "addr", cfg.RedisAddr)
	} else {
		deduper = dedup.NewMemory(cfg.DedupEnabled, dedupTTL)
		logger.Info("using in-memory deduplication (single-instance only)")
	}

	var pgPool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Error("failed to connect to postgres database", "error", err)
			os.Exit(1)
		}
		defer pool.Close()
		pgPool = pool
		logger.Info("connected to postgres database")
	}

	if cfg.FailedStoreEnabled {
		if pgPool != nil {
			failedStore = failstore.NewPostgresStore(pgPool)
		} else {
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
	}

	var replayAuditStore replayaudit.Store
	if cfg.ReplayAuditEnabled {
		if pgPool != nil {
			replayAuditStore = replayaudit.NewPostgresStore(pgPool)
		} else {
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
	}

	var securityAuditStore securityaudit.Store
	if cfg.SecurityAuditEnabled {
		if pgPool != nil {
			securityAuditStore = securityaudit.NewPostgresStore(pgPool)
		} else {
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
	}

	if isProductionLike(cfg.Environment) && pgPool == nil && (cfg.FailedStoreEnabled || cfg.ReplayAuditEnabled || cfg.SecurityAuditEnabled) {
		logger.Warn("production-like environment is using file-backed operational stores; set DATABASE_URL for durable multi-replica failed-event, replay-audit, and security-audit storage",
			"environment", cfg.Environment,
			"failed_event_store_enabled", cfg.FailedStoreEnabled,
			"replay_audit_enabled", cfg.ReplayAuditEnabled,
			"security_audit_enabled", cfg.SecurityAuditEnabled,
		)
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
		in.Source = source

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

	flags := config.NewFeatureFlags()
	flags.Register("dedup.enabled", cfg.DedupEnabled)
	flags.Register("rate_limit.enabled", cfg.RateLimitEnabled)
	flags.Register("schema_validation.enabled", cfg.SchemaValidationEnabled)
	flags.Register("retry.enabled", cfg.RetryEnabled)
	flags.Register("source_rate_limit.enabled", cfg.SourceRateLimitEnabled)
	flags.Register("queue_bulkhead.enabled", cfg.QueueBulkheadEnabled)

	admin := handlers.NewAdminHandlerWithDependencies(cfg, runtimePublisher.Publisher, logger, failedStore, replayAuditStore, securityAuditStore, flags)

	var schemaValidator *schema.Validator
	if cfg.SchemaValidationEnabled {
		var err error
		schemaValidator, err = schema.NewValidator(cfg.SchemaPath)
		if err != nil {
			logger.Error("schema validation disabled due to invalid configuration", "error", err)
		} else {
			logger.Info("schema validation enabled", "path", cfg.SchemaPath)
		}
	}

	h := handlers.NewWebhookHandlerWithDependencies(cfg, runtimePublisher.Publisher, logger, func(reqCtx context.Context, source string, status int) {
		telemetry.WebhookCounter.Add(reqCtx, 1,
			metric.WithAttributes(
				attribute.String("source", source),
				attribute.Int("status", status),
			),
		)
	}, deduper, failedStore, func(r *http.Request, event handlers.SecurityEvent) {
		record := handlers.SecurityAuditRecord(r, event)
		record.ClientIP = httpx.ClientIP(r, cfg.TrustedProxyCIDRs)
		recordSecurityEvent(r, record)
	}, schemaValidator)
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

	healthChecker := handlers.NewHealthChecker(runtimePublisher.Publisher, failedStore, deduper)

	var retryScheduler *retry.Scheduler
	if cfg.RetryEnabled && failedStore != nil {
		var leaderElection *retry.LeaderElection
		if redisClient != nil {
			hostname, _ := os.Hostname()
			if hostname == "" {
				hostname = fmt.Sprintf("instance-%d", time.Now().UnixNano())
			}
			leaderElection = retry.NewLeaderElection(redisClient, "teampulse:retry_leader", hostname, 2*time.Minute)
			logger.Info("retry scheduler leader election enabled via redis", "instance_id", hostname)
		}

		retryScheduler = retry.NewScheduler(failedStore, runtimePublisher.Publisher, logger, retry.SchedulerOptions{
			MaxRetries:     cfg.RetryMaxAttempts,
			Interval:       time.Duration(cfg.RetryIntervalSec) * time.Second,
			LeaderElection: leaderElection,
			OnRetry: func(ctx context.Context, source string, success bool, attempt int) {
				telemetry.QueuePublishCounter.Add(ctx, 1,
					metric.WithAttributes(
						attribute.String("source", source),
						attribute.String("result", map[bool]string{true: "retry_success", false: "retry_failed"}[success]),
						attribute.Int("attempt", attempt),
					),
				)
			},
		})
		retryScheduler.Start()
		defer retryScheduler.Stop()
	}

	webhookMux := http.NewServeMux()
	webhookMux.HandleFunc("GET /{$}", handlers.ProductUI)
	webhookMux.HandleFunc("GET /assets/ui.css", handlers.ProductUIStyles)
	webhookMux.HandleFunc("GET /assets/ui.js", handlers.ProductUIScript)
	webhookMux.HandleFunc("GET /healthz", healthChecker.Healthz)
	webhookMux.HandleFunc("GET /readyz", healthChecker.Readyz)
	webhookMux.Handle("GET /metrics", telemetry.MetricsHandler)
	webhookMux.HandleFunc("GET /admin/ui", admin.HandleAdminUI)
	webhookMux.HandleFunc("GET /assets/admin.css", admin.AdminUIStyles)
	webhookMux.HandleFunc("GET /assets/admin.js", admin.AdminUIScript)
	webhookMux.HandleFunc("GET /admin/configz", admin.Configz)
	webhookMux.HandleFunc("GET /admin/events/failed", admin.FailedEvents)
	webhookMux.HandleFunc("GET /admin/events/replay-audit", admin.ReplayAudit)
	webhookMux.HandleFunc("GET /admin/events/security-audit", admin.SecurityAudit)
	webhookMux.HandleFunc("GET /admin/flags", admin.FeatureFlags)
	webhookMux.HandleFunc("POST /admin/flags", admin.FeatureFlags)
	webhookMux.HandleFunc("POST /admin/events/replay/batch", admin.ReplayFailedEventsBatch)
	webhookMux.HandleFunc("POST /admin/events/replay", admin.ReplayFailedEvent)
	webhookHandlers := map[string]http.HandlerFunc{
		"slack":  h.HandleSlack,
		"teams":  h.HandleTeams,
		"github": h.HandleGitHub,
		"gitlab": h.HandleGitLab,
	}

	httpx.RegisterVersionedRoutes(webhookMux, webhookHandlers, "/api/v1/webhooks/")
	httpx.RegisterVersionedRoutes(webhookMux, webhookHandlers, "/webhooks/")

	var requestLimiter httpx.RateLimiter
	var stopRequestLimiter func()
	if cfg.RateLimitBackend == "redis" {
		requestLimiter = httpx.NewRedisRateLimiter(redisClient, cfg.RateLimitRedisPrefix, time.Minute)
		logger.Info("using redis-backed request rate limiting", "addr", cfg.RedisAddr, "prefix", cfg.RateLimitRedisPrefix)
	} else {
		memoryLimiter := httpx.NewIPRateLimiter(nil, time.Minute, 1024)
		requestLimiter = memoryLimiter
		stopRequestLimiter = memoryLimiter.Stop
		logger.Info("using in-memory request rate limiting")
	}

	handlerBuilder := NewHandlerBuilderWithLimiter(logger, &cfg, securityRejectFn, requestLimiter, stopRequestLimiter)

	if configFile := os.Getenv("CONFIG_FILE"); configFile != "" {
		watcher, err := config.NewWatcher(cfg, logger, func(newCfg config.Config) {
			logger.Info("configuration updated", "path", configFile)
		})
		if err != nil {
			logger.Warn("config watcher initialization failed", "path", configFile, "error", err)
		} else {
			if err := watcher.Start(configFile); err != nil {
				logger.Warn("config watcher start failed", "path", configFile, "error", err)
			} else {
				defer func() {
					if stopErr := watcher.Stop(); stopErr != nil {
						logger.Error("config watcher stop failed", "error", stopErr)
					}
				}()
			}
		}
	}

	handlerWrapper := &deferredHandler{}
	uiSmokeProxy := handlers.NewUISmokeTestProxy(handlerWrapper, cfg.TrustedProxyCIDRs)
	publicMux := newPublicMux(webhookMux, uiSmokeProxy)
	handler := handlerBuilder.Build(publicMux, telemetry.HTTPDurationHistogram)
	handlerWrapper.set(handler)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		certFile := envOrTLS("TLS_CERT_FILE")
		keyFile := envOrTLS("TLS_KEY_FILE")
		if certFile != "" && keyFile != "" {
			logger.Info("ingestion-gateway listening (TLS)", "port", cfg.Port)
			if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				logger.Error("server failed", "error", err)
				serverErr <- err
			}
		} else {
			logger.Info("ingestion-gateway listening", "port", cfg.Port)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("server failed", "error", err)
				serverErr <- err
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stop:
		signal.Stop(stop)
	case err := <-serverErr:
		logger.Error("server error triggered shutdown", "error", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}

	deduper.Stop()
	handlerBuilder.Stop()
}

func isProductionLike(environment string) bool {
	env := strings.ToLower(strings.TrimSpace(environment))
	if env == "" {
		return false
	}
	if strings.Contains(env, "dev") || strings.Contains(env, "test") || strings.Contains(env, "local") || strings.Contains(env, "ci") {
		return false
	}
	return env == "prod" || env == "production" || strings.Contains(env, "prod")
}

func securitySourceFromPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/admin"):
		return "admin"
	case strings.HasPrefix(path, "/api/v1/webhooks/"):
		source := strings.TrimPrefix(path, "/api/v1/webhooks/")
		source = strings.Trim(source, "/")
		if source != "" {
			return source
		}
	case strings.HasPrefix(path, "/webhooks/"):
		source := strings.TrimPrefix(path, "/webhooks/")
		source = strings.Trim(source, "/")
		if source != "" {
			return source
		}
	}
	return "http"
}

func envOrTLS(key string) string {
	v := os.Getenv(key)
	if v == "" {
		return ""
	}
	if _, err := os.Stat(v); err != nil {
		return ""
	}
	return v
}
