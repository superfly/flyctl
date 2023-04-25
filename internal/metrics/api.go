package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/terminal"
)

const (
	timeout = 6 * time.Second
)

var (
	timedOut = atomic.Bool{}
	done     = sync.WaitGroup{}
)

func rawSendImpl(parentCtx context.Context, metricSlug, jsonValue string) error {

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
	req.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Version().String()))
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

func rawSend(parentCtx context.Context, metricSlug, jsonValue string) {

	if buildinfo.IsDev() {
		return
	}

	done.Add(1)
	go func() {
		defer done.Done()
		handleErr(rawSendImpl(parentCtx, metricSlug, jsonValue))
	}()
}

func FlushPending() {
	done.Wait()
}
