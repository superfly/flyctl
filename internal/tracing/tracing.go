package tracing

import (
	"context"
	"os"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/buildinfo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "github.com/superfly/flyctl"
)

func GetTracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

func RecordError(span trace.Span, err error, description string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, description)
}

// GetCMDSpan creates a new span with some defaults setup
func GetCMDSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	appName := appconfig.NameFromContext(ctx)
	startOpts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("app.name", appName),
		),
	}

	return GetTracer().Start(ctx, spanName, startOpts...)
}

func InitTraceProvider(ctx context.Context) (*sdktrace.TracerProvider, error) {
	var exporter sdktrace.SpanExporter
	switch {
	case os.Getenv("LOG_LEVEL") == "trace":
		stdoutExp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, err
		}
		exporter = stdoutExp
	}

	resourceAttrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String("flyctl"),
		attribute.String("build.info.version", buildinfo.Version().String()),
		attribute.String("build.info.os", buildinfo.OS()),
		attribute.String("build.info.arch", buildinfo.Arch()),
		attribute.String("build.info.commit", buildinfo.Commit()),
	}

	resource := resource.NewWithAttributes(
		semconv.SchemaURL,
		resourceAttrs...,
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	return tp, nil
}
