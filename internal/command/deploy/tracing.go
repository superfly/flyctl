package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flyutil"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func recordDeployIdentity(ctx context.Context, app *flaps.App) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("org.id", fmt.Sprint(app.Organization.InternalNumericID)),
		attribute.String("org.slug", app.Organization.Slug),
	)

	client := flyutil.ClientFromContext(ctx)
	if client == nil {
		return
	}

	// Identity enrichment must not prevent deployment or wait indefinitely.
	ctx, cancel := context.WithTimeoutCause(ctx, 2*time.Second, fmt.Errorf("fetching deployment trace identity: %w", context.DeadlineExceeded))
	defer cancel()
	user, err := client.GetCurrentUser(ctx)
	if err != nil || user == nil {
		return
	}
	if user.ID != "" {
		span.SetAttributes(attribute.String("user.id", user.ID))
	}
	if user.Email != "" {
		span.SetAttributes(attribute.String("user.email", user.Email))
	}
}
