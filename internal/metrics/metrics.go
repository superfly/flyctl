package metrics

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/terminal"
)

var Enabled = true

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
	var (
		store  = StoreFromContext(ctx)
		logger = logger.FromContext(ctx)
	)

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

	entry := &Entry{
		Metric:    metricSlug + "/started",
		Timestamp: time.Now(),
	}

	if _, err := store.Write(entry); err != nil {
		logger.Debugf("failed to write metrics: %v", err)
	}
}
func Status(ctx context.Context, metricSlug string, success bool) {
	var (
		store  = StoreFromContext(ctx)
		logger = logger.FromContext(ctx)
	)

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

	data, err := json.Marshal(map[string]bool{"success": success})
	if err != nil {
		logger.Debugf("failed to encode data: %v", err)
	}

	entry := &Entry{
		Metric:    metricSlug + "/status",
		Payload:   data,
		Timestamp: time.Now(),
	}
	if _, err := store.Write(entry); err != nil {
		logger.Debugf("failed to write metrics: %v", err)
	}
}

func Save[T any](ctx context.Context, metricSlug string, value T) {
	var (
		store  = StoreFromContext(ctx)
		logger = logger.FromContext(ctx)
	)

	valJson, err := json.Marshal(value)
	if err != nil {
		return
	}

	entry := &Entry{
		Metric:    metricSlug,
		Payload:   valJson,
		Timestamp: time.Now(),
	}
	if _, err := store.Write(entry); err != nil {
		logger.Debugf("failed to write metrics: %v", err)
	}
}

func SaveNoData(ctx context.Context, metricSlug string) {
	SaveJSON(ctx, metricSlug, nil)
}

func SaveJSON(ctx context.Context, metricSlug string, payload json.RawMessage) {
	var (
		store  = StoreFromContext(ctx)
		logger = logger.FromContext(ctx)
	)

	entry := &Entry{
		Metric:    metricSlug,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	if _, err := store.Write(entry); err != nil {
		logger.Debugf("failed to write metrics: %v", err)
	}
}

func StartTiming(ctx context.Context, metricSlug string) func() {
	start := time.Now()
	return func() {
		Save(ctx, metricSlug, map[string]float64{"duration_seconds": time.Since(start).Seconds()})
	}
}

func ShouldSendMetrics(ctx context.Context) bool {
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
