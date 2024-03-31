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
