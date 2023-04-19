package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/terminal"
)

const (
	timeout = 4 * time.Second
)

var timedOut = atomic.Bool{}

func sendImpl(parentCtx context.Context, metricSlug, jsonValue string) error {

	token, err := getMetricsToken(parentCtx)
	if err != nil {
		return err
	}

	cfg := config.FromContext(parentCtx)

	if timedOut.Load() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	reader := strings.NewReader(jsonValue)

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.MetricsBaseURL+"/v1/"+metricSlug, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	resp, err := http.DefaultClient.Do(req)
	defer func() {
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()
	if ctx.Err() == context.DeadlineExceeded {
		timedOut.Store(true)
		return nil
	}
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metrics server returned status code %d", resp.StatusCode)
	}
	return nil
}

func handleErr(err error) {
	if err == nil {
		return
	}
	// TODO(ali): Should this ping sentry when it fails?
	terminal.Debugf("metrics error: %v", err)
}

func Started(ctx context.Context, metricSlug string) {
	SendNoData(ctx, metricSlug+"/started")
}
func Status(ctx context.Context, metricSlug string, success bool) {
	Send(ctx, metricSlug+"/status", map[string]bool{"success": success})
}

func Send[T any](ctx context.Context, metricSlug string, value T) {

	valJson, err := json.Marshal(value)
	if err != nil {
		return
	}
	SendJson(ctx, metricSlug, string(valJson))
}

func SendNoData(ctx context.Context, metricSlug string) {

	SendJson(ctx, metricSlug, "")
}

func SendJson(ctx context.Context, metricSlug, jsonValue string) {
	handleErr(sendImpl(ctx, metricSlug, jsonValue))
}

func StartTiming(ctx context.Context, metricSlug string) func() {
	start := time.Now()
	return func() {
		Send(ctx, metricSlug+"/duration", map[string]float64{"duration_seconds": time.Since(start).Seconds()})
	}
}
