package tracing

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/go-logr/logr"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
)

var tp *sdktrace.TracerProvider

const (
	tracerName       = "github.com/superfly/flyctl"
	HeaderFlyTraceId = "fly-trace-id"
	HeaderFlySpanId  = "fly-span-id"
)

func getCollectorUrl() string {
	url := os.Getenv("FLY_TRACE_COLLECTOR_URL")
	if url != "" {
		return url
	}

	if buildinfo.IsDev() {
		return "fly-otel-collector-dev.fly.dev"
	}

	return "fly-otel-collector-prod.fly.dev"
}

func GetTracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

func RecordError(span trace.Span, err error, description string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, description)
}

func CreateLinkSpan(ctx context.Context, res *http.Response) {
	remoteSpanCtx := SpanContextFromHeaders(res)
	_, span := GetTracer().Start(ctx, "flaps.link", trace.WithLinks(trace.Link{SpanContext: remoteSpanCtx}))
	defer span.End()
}

func SpanContextFromHeaders(res *http.Response) trace.SpanContext {
	traceIDstr := res.Header.Get(HeaderFlyTraceId)
	spanIDstr := res.Header.Get(HeaderFlySpanId)

	traceID, _ := trace.TraceIDFromHex(traceIDstr)
	spanID, _ := trace.SpanIDFromHex(spanIDstr)

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})
}

func CMDSpan(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	startOpts := []trace.SpanStartOption{
		trace.WithSpanKind(trace.SpanKindClient),
	}

	startOpts = append(startOpts, opts...)

	return GetTracer().Start(ctx, spanName, startOpts...)
}

func getToken(ctx context.Context) string {
	token := config.Tokens(ctx).Flaps()
	if token == "" {
		token = os.Getenv("FLY_API_TOKEN")
	}
	return token
}

func InitTraceProviderWithoutApp(ctx context.Context) (*sdktrace.TracerProvider, error) {
	return InitTraceProvider(ctx, "")
}

func InitTraceProvider(ctx context.Context, appName string) (*sdktrace.TracerProvider, error) {
	if tp != nil {
		return tp, nil
	}

	var exporter sdktrace.SpanExporter
	switch {
	case os.Getenv("LOG_LEVEL") == "trace":
		stdoutExp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, err
		}
		exporter = stdoutExp

	default:

		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(getCollectorUrl() + ":4318"),
			otlptracehttp.WithInsecure(),
			otlptracehttp.WithHeaders(map[string]string{
				"authorization": getToken(ctx),
			}),
		}
		httpExporter, err := otlptracehttp.New(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to telemetry collector")
		}

		exporter = httpExporter
	}

	resourceAttrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String("flyctl"),
		attribute.String("build.info.version", buildinfo.Version().String()),
		attribute.String("build.info.os", buildinfo.OS()),
		attribute.String("build.info.arch", buildinfo.Arch()),
		attribute.String("build.info.commit", buildinfo.Commit()),
		attribute.Bool("is_ci", env.IsCI()),
	}

	if appName != "" {
		resourceAttrs = append(resourceAttrs, attribute.String("app.name", appName))
	}

	resource := resource.NewWithAttributes(
		semconv.SchemaURL,
		resourceAttrs...,
	)

	tp = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	otel.SetLogger(otelLogger(ctx))

	otel.SetErrorHandler(errorHandler(ctx))

	return tp, nil
}

func otelLogger(ctx context.Context) logr.Logger {
	io := iostreams.FromContext(ctx)

	var level slog.Level
	switch os.Getenv("LOG_LEVEL") {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}
	handler := slog.NewTextHandler(io.ErrOut, opts)

	return logr.FromSlogHandler(handler)
}

func errorHandler(ctx context.Context) otel.ErrorHandler {
	logger := logger.FromContext(ctx)

	return otel.ErrorHandlerFunc(func(err error) {
		logger.Debug("trace exporter", "error", err)
	})
}
