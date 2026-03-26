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
)

type Telemetry struct {
	MetricsHandler        http.Handler
	WebhookCounter        metric.Int64Counter
	SecurityRejectCounter metric.Int64Counter
	shutdown              []func(context.Context) error
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	for i := len(t.shutdown) - 1; i >= 0; i-- {
		if err := t.shutdown[i](ctx); err != nil {
			return err
		}
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

	logger.Info("telemetry initialized", "otel_http_instrumentation", true)
	return telemetry, nil
}

func HTTPMiddleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(next, serviceName)
	}
}

func setupTracing(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))))
		return tp, tp.Shutdown, nil
	}

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, nil, fmt.Errorf("init otlp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.2))),
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
