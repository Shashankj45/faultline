package telemetry

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	TracerName = "github.com/faultline-proxy/faultline"
)

// InitTracer initializes OpenTelemetry tracing for FaultLine.
func InitTracer(serviceName string) (*sdktrace.TracerProvider, func(context.Context) error) {
	// Configure W3C TraceContext propagator globally
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)

	return tp, tp.Shutdown
}

// Tracer returns the named tracer instance.
func Tracer() trace.Tracer {
	return otel.GetTracerProvider().Tracer(TracerName)
}

// StartProxySpan starts a span for reverse proxy request ingress, extracting incoming traceparent if present.
func StartProxySpan(r *http.Request, upstreamSlug string) (context.Context, trace.Span) {
	propagator := otel.GetTextMapPropagator()
	ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

	ctx, span := Tracer().Start(ctx, "FaultLine.ProxyIngress",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			semconv.HTTPRequestMethodKey.String(r.Method),
			semconv.URLPathKey.String(r.URL.Path),
			attribute.String("upstream.slug", upstreamSlug),
			attribute.String("client.ip", r.RemoteAddr),
		),
	)

	// Inject trace context back into request headers so upstream receives traceparent
	propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))

	return ctx, span
}

// RecordFaultInjection adds chaos injection metadata to the active span.
func RecordFaultInjection(span trace.Span, faultType string, expID string, probability float64) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.Bool("chaos.injected", true),
		attribute.String("chaos.fault_type", faultType),
		attribute.String("chaos.experiment_id", expID),
		attribute.Float64("chaos.probability", probability),
	)
	span.AddEvent("chaos_fault_injected", trace.WithAttributes(
		attribute.String("fault.type", faultType),
		attribute.String("experiment.id", expID),
	))
}
