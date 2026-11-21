package tracing

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Ensure phase and command spans export only fixed codes, including wrapped,
// canceled, mixed, and unknown failures, while preserving errors for the caller.
func TestWorkflowErrorTelemetry(t *testing.T) {
	private := errors.New("postgres://private-user:secret-password@private-host/private-db")
	tagged := WithWorkflowErrorCode(private, WorkflowErrorSSHNoStartedMachines)
	for _, tt := range []struct {
		name string
		err  error
		code string
	}{
		{"source code", tagged, "ssh_no_started_machines"},
		{"wrapped source code", fmt.Errorf("private-machine-id: %w", tagged), "ssh_no_started_machines"},
		{"unrecognized error", private, "unknown"},
		{"canceled", fmt.Errorf("private-host: %w", context.Canceled), "canceled"},
		{"deadline", fmt.Errorf("private-host: %w", context.DeadlineExceeded), "deadline_exceeded"},
		{"mixed causes", fmt.Errorf("private-host: %w", errors.Join(tagged, private)), "unknown"},
		{"same causes", errors.Join(tagged, tagged), "ssh_no_started_machines"},
		{"invalid code", WithWorkflowErrorCode(private, WorkflowErrorCode(999)), "unknown"},
		{"negative code", WithWorkflowErrorCode(private, WorkflowErrorCode(-1)), "unknown"},
		{"success", WithWorkflowErrorCode(nil, WorkflowErrorSSHNoStartedMachines), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previousProvider, previousTP := otel.GetTracerProvider(), tp
			otel.SetTracerProvider(provider)
			tp = provider
			t.Cleanup(func() {
				otel.SetTracerProvider(previousProvider)
				tp = previousTP
				_ = provider.Shutdown(context.Background())
			})
			got := RunWorkflow(context.Background(), "ssh.console", "private-app", func(ctx context.Context) error {
				return WorkflowPhase(ctx, "select_machine", func(ctx context.Context) error {
					return tt.err
				})
			})
			if got != tt.err {
				t.Fatalf("returned error changed: %v", got)
			}
			spans := recorder.Ended()
			if len(spans) != 2 {
				t.Fatalf("expected phase and command: %d spans", len(spans))
			}
			for _, span := range spans {
				if value := attrValue(span.Attributes(), "workflow.error.code"); value != tt.code {
					t.Fatalf("%s code=%q, want %q", span.Name(), value, tt.code)
				}
				if len(span.Events()) != 0 || len(span.Attributes()) != boolInt(tt.err != nil) {
					t.Fatalf("unexpected diagnostic payload: attrs=%v events=%v", span.Attributes(), span.Events())
				}
				if tt.err != nil && (span.Status().Code != codes.Error || span.Status().Description != "workflow operation failed") {
					t.Fatalf("failure status changed: %v", span.Status())
				}
				if tt.err == nil && span.Status().Code != codes.Ok {
					t.Fatalf("successful workflow failed: %v", span.Status())
				}
			}
		})
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// Ensure tagging preserves error text, errors.Is, and errors.As for existing
// command handlers, and completed phases cannot be changed by later cleanup.
func TestWorkflowErrorChainAndCompletion(t *testing.T) {
	original := &workflowTestError{message: "private diagnostic"}
	err := WithWorkflowErrorCode(original, WorkflowErrorSSHMachineList)
	var target *workflowTestError
	if err.Error() != original.Error() || !errors.Is(err, original) || !errors.As(err, &target) || target != original {
		t.Fatalf("tagging changed the original error: %v", err)
	}
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	ctx := context.WithValue(context.Background(), workflowContextKey{}, func(error) {})
	_, finish := StartWorkflowPhase(ctx, "open_session")
	finish(nil)
	finish(err)
	span := recorder.Ended()[0]
	if span.Status().Code != codes.Ok || len(span.Attributes()) != 0 || len(span.Events()) != 0 {
		t.Fatalf("cleanup changed a successful session: %v", span)
	}
}

type workflowTestError struct{ message string }

func (err *workflowTestError) Error() string { return err.message }
