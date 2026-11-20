package metrics

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/superfly/flyctl/internal/config"
)

// metricsEnabledContext returns a context metrics are sent from. The base URL
// is deliberately not the production one: dev builds refuse to report there.
func metricsEnabledContext() context.Context {
	return config.NewContext(context.Background(), &config.Config{
		SendMetrics:    true,
		MetricsBaseURL: "http://metrics.example.invalid",
	})
}

// withEmptyQueue runs fn against a fresh queue and returns what it queued.
func withEmptyQueue(t *testing.T, fn func()) []metricsMessage {
	t.Helper()

	saved := metrics
	metrics = make([]metricsMessage, 0)
	defer func() { metrics = saved }()

	fn()

	return metrics
}

func TestStartImagePushTimingLabelsTheBuilder(t *testing.T) {
	queued := withEmptyQueue(t, func() {
		StartImagePushTiming(metricsEnabledContext(), "remote", "iad")()
	})

	if len(queued) != 1 {
		t.Fatalf("queued %d metrics, want 1", len(queued))
	}
	if got, want := queued[0].Metric, "image_push/duration"; got != want {
		t.Errorf("metric slug = %q, want %q", got, want)
	}

	var payload ImagePushPayload
	if err := json.Unmarshal(queued[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if got, want := payload.BuilderType, "remote"; got != want {
		t.Errorf("builder_type = %q, want %q", got, want)
	}
	if got, want := payload.BuilderRegion, "iad"; got != want {
		t.Errorf("builder_region = %q, want %q", got, want)
	}
	if payload.DurationSeconds < 0 {
		t.Errorf("duration_seconds = %v, want a non-negative duration", payload.DurationSeconds)
	}
}

// Local and depot builds have no region. Those labels are omitted rather than
// reported empty, so the collector doesn't see "" as its own bucket.
func TestStartImagePushTimingOmitsUnknownLabels(t *testing.T) {
	queued := withEmptyQueue(t, func() {
		StartImagePushTiming(metricsEnabledContext(), "", "")()
	})

	if len(queued) != 1 {
		t.Fatalf("queued %d metrics, want 1", len(queued))
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(queued[0].Payload, &fields); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if _, ok := fields["duration_seconds"]; !ok {
		t.Error("payload has no duration_seconds")
	}
	for _, key := range []string{"builder_type", "builder_region"} {
		if _, ok := fields[key]; ok {
			t.Errorf("payload has %s, want it omitted when unknown", key)
		}
	}
}
