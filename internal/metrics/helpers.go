package metrics

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/superfly/flyctl/terminal"
)

var (
	unmatchedStatusesMtx = sync.Mutex{}
	unmatchedStatuses    = map[string]struct{}{}
)

func withUnmatchedStatuses[T any](cb func(map[string]struct{}) T) T {
	unmatchedStatusesMtx.Lock()
	defer unmatchedStatusesMtx.Unlock()
	return cb(unmatchedStatuses)
}

func Started(ctx context.Context, metricSlug string) {
	ok := withUnmatchedStatuses(func(unmatchedStatuses map[string]struct{}) bool {
		if _, ok := unmatchedStatuses[metricSlug]; ok {
			return false
		}
		unmatchedStatuses[metricSlug] = struct{}{}
		return true
	})
	if !ok {
		terminal.Debugf("Metrics: Attempted to send start event for %s, but it was already started", metricSlug)
		return
	}

	SendNoData(ctx, metricSlug+"/started")

}
func Status(ctx context.Context, metricSlug string, success bool) {
	ok := withUnmatchedStatuses(func(unmatchedStatuses map[string]struct{}) bool {
		if _, ok := unmatchedStatuses[metricSlug]; ok {
			delete(unmatchedStatuses, metricSlug)
			return true
		}
		return false
	})
	if !ok {
		terminal.Debugf("Metrics: Attempted to send status for %s, but no start event was sent", metricSlug)
		return
	}

	Send(ctx, metricSlug+"/status", map[string]bool{"success": success})
}

func Send[T any](ctx context.Context, metricSlug string, value T) {

	valJson, err := json.Marshal(value)
	if err != nil {
		return
	}
	SendJson(ctx, metricSlug, valJson)
}

func SendNoData(ctx context.Context, metricSlug string) {
	SendJson(ctx, metricSlug, nil)
}

func SendJson(ctx context.Context, metricSlug string, payload json.RawMessage) {
	rawSend(ctx, metricSlug, payload)
}

func StartTiming(ctx context.Context, metricSlug string) func() {
	start := time.Now()
	return func() {
		Send(ctx, metricSlug, map[string]float64{"duration_seconds": time.Since(start).Seconds()})
	}
}

type disableFlushMetricsKey struct{}

// WithDisableFlushMetrics returns a context with a flag that disables
// FlushMetrics call after a command completes.
func WithDisableFlushMetrics(ctx context.Context) context.Context {
	return context.WithValue(ctx, disableFlushMetricsKey{}, true)
}

// IsFlushMetricsDisabled returns true if FlushMetrics should not be called after a command completes.
func IsFlushMetricsDisabled(ctx context.Context) bool {
	if _, ok := os.LookupEnv("FLY_NO_METRICS"); ok {
		return false
	}

	val, ok := ctx.Value(disableFlushMetricsKey{}).(bool)
	if !ok {
		return false
	}

	return val
}
