package otel

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "llm-gateway"

// Setup initializes the OTel tracer provider and W3C TraceContext propagator.
// If OTEL_EXPORTER_OTLP_ENDPOINT is not set, spans are created but not exported.
// Default HTTP endpoint: http://localhost:4318 (OTLP/HTTP); set via OTEL_EXPORTER_OTLP_ENDPOINT.
func Setup(ctx context.Context, log *slog.Logger) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	serviceName := os.Getenv("OTEL_SERVICE_NAME")
	if serviceName == "" {
		serviceName = "llm-gateway"
	}

	// W3C TraceContext (traceparent/tracestate) + Baggage propagators.
	// Must be set before any span creation so incoming distributed traces are continued.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, err
	}

	if endpoint == "" {
		tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
		otel.SetTracerProvider(tp)
		log.Info("otel: no exporter endpoint configured, traces will not be exported")
		return tp.Shutdown, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	log.Info("otel: tracer provider initialized", "endpoint", endpoint,
		"protocol", "otlp/http",
		"propagator", "W3C TraceContext + Baggage")

	return tp.Shutdown, nil
}

// PropagationMiddleware extracts the W3C traceparent (and baggage) from incoming
// HTTP request headers and injects the resulting span context into the request context.
// Must wrap all handlers so distributed traces from upstream services are continued.
func PropagationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Tracer returns the gateway's tracer.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// Common attribute helpers for spans.
func AttrTenant(id string) attribute.KeyValue {
	return attribute.String("tenant.id", id)
}

func AttrModel(name string) attribute.KeyValue {
	return attribute.String("model.name", name)
}

func AttrProvider(name string) attribute.KeyValue {
	return attribute.String("provider.name", name)
}

func AttrAttempt(n int) attribute.KeyValue {
	return attribute.Int("attempt", n)
}

func AttrStatus(code int) attribute.KeyValue {
	return attribute.Int("http.status_code", code)
}

func AttrAuthType(authType string) attribute.KeyValue {
	return attribute.String("auth.type", authType)
}

func AttrSub(sub string) attribute.KeyValue {
	return attribute.String("auth.sub", sub)
}
