package metrics

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
)

var (
	Enabled = true
	done    sync.WaitGroup
)

type metricsMessage struct {
	Metric  string          `json:"m"`
	Payload json.RawMessage `json:"p"`
}

func rawSend(parentCtx context.Context, metricSlug string, payload json.RawMessage) {
	if !shouldSendMetrics(parentCtx) {
		return
	}

	message := metricsMessage{
		Metric:  metricSlug,
		Payload: payload,
	}

	queueMetric(message)
}

// SendImmediate sends a single metric to the collector right away instead of
// queueing it for the end-of-command flush. Long-running processes (e.g. the
// agent) queue metrics that would otherwise never be flushed.
func SendImmediate[T any](ctx context.Context, metricSlug string, value T) {
	if !shouldSendMetrics(ctx) {
		return
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return
	}

	buf, err := json.Marshal([]metricsMessage{{
		Metric:  metricSlug,
		Payload: payload,
	}})
	if err != nil {
		return
	}

	done.Add(1)
	go func() {
		defer done.Done()

		_ = SendMetrics(ctx, string(buf))
	}()
}

func shouldSendMetrics(ctx context.Context) bool {
	if !Enabled {
		return false
	}

	cfg := config.FromContext(ctx)

	if !cfg.SendMetrics {
		return false
	}

	// never send metrics to the production collector from dev builds
	if buildinfo.IsDev() && cfg.MetricsBaseURLIsProduction() {
		return false
	}

	return true
}

func FlushPending() {
	if !Enabled {
		return
	}

	done.Wait()
}
