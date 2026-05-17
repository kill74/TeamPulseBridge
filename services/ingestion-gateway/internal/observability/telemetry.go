package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	prometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"teampulsebridge/services/ingestion-gateway/internal/queue"
)

type Telemetry struct {
	MetricsHandler           http.Handler
	WebhookCounter           metric.Int64Counter
	SecurityRejectCounter    metric.Int64Counter
	QueueBackpressureCounter metric.Int64Counter
	QueuePublishCounter      metric.Int64Counter
	HTTPDurationHistogram    metric.Float64Histogram
	QueuePublishLatency      metric.Float64Histogram
	shutdown                 []func(context.Context) error
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for i := len(t.shutdown) - 1; i >= 0; i-- {
		if err := t.shutdown[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("telemetry shutdown errors: %v", errs)
	}
	return nil
}

func Setup(ctx context.Context, logger *slog.Logger, serviceName string) (*Telemetry, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			attribute.String("deployment.environment", envOr("ENVIRONMENT", "dev")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	telemetry := &Telemetry{}

	traceProvider, traceShutdown, err := setupTracing(ctx, res)
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(traceProvider)
	telemetry.shutdown = append(telemetry.shutdown, traceShutdown)

	meterProvider, metricsHandler, metricsShutdown, err := setupMetrics(res)
	if err != nil {
		return nil, err
	}
	otel.SetMeterProvider(meterProvider)
	telemetry.MetricsHandler = metricsHandler
	telemetry.shutdown = append(telemetry.shutdown, metricsShutdown)

	counter, err := meterProvider.Meter(serviceName).Int64Counter("webhook_requests_total")
	if err != nil {
		return nil, fmt.Errorf("create metric counter: %w", err)
	}
	telemetry.WebhookCounter = counter

	securityRejectCounter, err := meterProvider.Meter(serviceName).Int64Counter("security_rejections_total")
	if err != nil {
		return nil, fmt.Errorf("create security reject counter: %w", err)
	}
	telemetry.SecurityRejectCounter = securityRejectCounter

	queueBackpressureCounter, err := meterProvider.Meter(serviceName).Int64Counter("queue_backpressure_events_total")
	if err != nil {
		return nil, fmt.Errorf("create queue backpressure counter: %w", err)
	}
	telemetry.QueueBackpressureCounter = queueBackpressureCounter

	queuePublishCounter, err := meterProvider.Meter(serviceName).Int64Counter("queue_publish_outcomes_total")
	if err != nil {
		return nil, fmt.Errorf("create queue publish counter: %w", err)
	}
	telemetry.QueuePublishCounter = queuePublishCounter

	durationHistogram, err := meterProvider.Meter(serviceName).Float64Histogram(
		"http_request_duration_seconds",
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create http duration histogram: %w", err)
	}
	telemetry.HTTPDurationHistogram = durationHistogram

	queuePublishLatency, err := meterProvider.Meter(serviceName).Float64Histogram(
		"queue_publish_latency_seconds",
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1),
	)
	if err != nil {
		return nil, fmt.Errorf("create queue publish latency histogram: %w", err)
	}
	telemetry.QueuePublishLatency = queuePublishLatency

	logger.Info("telemetry initialized", "otel_http_instrumentation", true)
	return telemetry, nil
}

func (t *Telemetry) BindQueueMetrics(serviceName string, provider queue.SnapshotProvider) error {
	if provider == nil {
		return nil
	}
	meter := otel.GetMeterProvider().Meter(serviceName)

	usageGauge, err := meter.Float64ObservableGauge("queue_buffer_usage_ratio")
	if err != nil {
		return fmt.Errorf("create queue buffer usage gauge: %w", err)
	}
	failureGauge, err := meter.Float64ObservableGauge("queue_failure_budget_ratio")
	if err != nil {
		return fmt.Errorf("create queue failure budget gauge: %w", err)
	}
	depthGauge, err := meter.Int64ObservableGauge("queue_buffer_depth")
	if err != nil {
		return fmt.Errorf("create queue buffer depth gauge: %w", err)
	}
	sourceUsageGauge, err := meter.Float64ObservableGauge("queue_source_buffer_usage_ratio")
	if err != nil {
		return fmt.Errorf("create source queue buffer usage gauge: %w", err)
	}
	sourceFailureGauge, err := meter.Float64ObservableGauge("queue_source_failure_budget_ratio")
	if err != nil {
		return fmt.Errorf("create source queue failure budget gauge: %w", err)
	}
	sourceDepthGauge, err := meter.Int64ObservableGauge("queue_source_buffer_depth")
	if err != nil {
		return fmt.Errorf("create source queue buffer depth gauge: %w", err)
	}

	sourceProvider, _ := provider.(queue.SourceSnapshotProvider)
	registration, err := meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		snapshot := provider.Snapshot()
		observer.ObserveFloat64(usageGauge, snapshot.UsageRatio)
		observer.ObserveFloat64(failureGauge, snapshot.FailureRatio)
		observer.ObserveInt64(depthGauge, int64(snapshot.Depth))
		if sourceProvider != nil {
			for source, sourceSnapshot := range sourceProvider.SourceSnapshots() {
				attrs := metric.WithAttributes(attribute.String("source", source))
				observer.ObserveFloat64(sourceUsageGauge, sourceSnapshot.UsageRatio, attrs)
				observer.ObserveFloat64(sourceFailureGauge, sourceSnapshot.FailureRatio, attrs)
				observer.ObserveInt64(sourceDepthGauge, int64(sourceSnapshot.Depth), attrs)
			}
		}
		return nil
	}, usageGauge, failureGauge, depthGauge, sourceUsageGauge, sourceFailureGauge, sourceDepthGauge)
	if err != nil {
		return fmt.Errorf("register queue metrics callback: %w", err)
	}
	t.shutdown = append(t.shutdown, func(context.Context) error {
		return registration.Unregister()
	})
	return nil
}

func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, serviceName)
	}
}

func setupTracing(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
		)
		return tp, tp.Shutdown, nil
	}

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint))
	if err != nil {
		return nil, nil, fmt.Errorf("init otlp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
	)
	return tp, tp.Shutdown, nil
}

func setupMetrics(res *resource.Resource) (*sdkmetric.MeterProvider, http.Handler, func(context.Context) error, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("init prometheus exporter: %w", err)
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(exporter),
	)
	return provider, promhttp.Handler(), provider.Shutdown, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
