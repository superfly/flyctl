package metrics

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
)

var Enabled = true
var done sync.WaitGroup

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

	// this just waits for metrics to hit write(2) on the websocket connection
	// there is no need to wait on a response from the collector
	done.Wait()
}
