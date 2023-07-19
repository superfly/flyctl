package metrics

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/terminal"
)

var Enabled = true
var done sync.WaitGroup

type websocketMessage struct {
	Metric  string          `json:"m"`
	Payload json.RawMessage `json:"p"`
}

func handleErr(err error) {
	if err == nil {
		return
	}
	// TODO(ali): Should this ping sentry when it fails?
	terminal.Debugf("metrics error: %v\n", err)
}

func rawSend(parentCtx context.Context, metricSlug string, payload json.RawMessage) {
	if !shouldSendMetrics(parentCtx) {
		return
	}

	message := websocketMessage{
		Metric:  metricSlug,
		Payload: payload,
	}

	go func() {
		defer done.Done()
		handleErr(insertMetricToDB(message))
	}()

	// TODO(billy): Do we need this? probably not, right?
	done.Add(1)
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
