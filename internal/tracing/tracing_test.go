package tracing

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/sync/errgroup"
)

func TestRecordErrorCause(t *testing.T) {
	for _, kind := range []string{"deadline", "sibling", "cleanup", "active"} {
		t.Run(kind, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer provider.Shutdown(context.Background())
			parent, span := provider.Tracer("test").Start(context.Background(), "operation")
			var ctx context.Context
			var cause error
			switch kind {
			case "deadline":
				cause = fmt.Errorf("waiting for machine health: %w", context.DeadlineExceeded)
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeoutCause(parent, -time.Second, cause)
				defer cancel()
			case "sibling":
				cause = errors.New("machine update failed")
				group, child := errgroup.WithContext(parent)
				ctx = child
				group.Go(func() error { return cause })
				if err := group.Wait(); err != cause {
					t.Fatal(err)
				}
			case "cleanup":
				var cancel context.CancelCauseFunc
				ctx, cancel = context.WithCancelCause(parent)
				cancel(fmt.Errorf("log stream stopped: %w", context.Canceled))
				cause = context.Cause(ctx)
			default:
				ctx = parent
			}
			original := errors.New("request failed")
			RecordError(ctx, span, original, "operation failed")
			span.End()
			got := recorder.Ended()[0]
			if got.Status().Code != codes.Error || got.Status().Description != "operation failed" {
				t.Fatalf("status=%v", got.Status())
			}
			events := got.Events()
			if len(events) != 1 || attrValue(events[0].Attributes, "exception.message") != original.Error() {
				t.Fatalf("original error lost: %v", events)
			}
			want := ""
			if cause != nil {
				want = cause.Error()
			}
			if attrValue(got.Attributes(), "context.cause") != want || attrValue(events[0].Attributes, "context.cause") != want {
				t.Fatalf("cause missing: span=%v events=%v", got.Attributes(), events)
			}
		})
	}
}

func TestCleanupDoesNotFailSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer provider.Shutdown(context.Background())
	parent, span := provider.Tracer("test").Start(context.Background(), "operation")
	ctx, cancel := context.WithCancelCause(parent)
	cancel(fmt.Errorf("operation finished: %w", context.Canceled))
	RecordCancellation(ctx, span)
	RecordError(ctx, span, nil, "must not fail")
	span.End()
	got := recorder.Ended()[0]
	if got.Status().Code != codes.Unset || len(got.Events()) != 0 {
		t.Fatalf("cleanup recorded a failure: status=%v events=%v", got.Status(), got.Events())
	}
	if attrValue(got.Attributes(), "context.cause.type") != "canceled" {
		t.Fatalf("cancellation classification missing: %v", got.Attributes())
	}
}

func attrValue(attrs []attribute.KeyValue, key string) string {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}

	return ""
}
