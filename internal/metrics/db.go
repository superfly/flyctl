package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/iostreams"
)

var metrics []metricsMessage = make([]metricsMessage, 0)

func queueMetric(metric metricsMessage) {
	metrics = append(metrics, metric)
}

// Spawns a forked `flyctl metrics send` process that sends metrics to the flyctl-metrics server
func FlushMetrics(ctx context.Context) error {
	if len(metrics) == 0 {
		// Don't bother sending an empty request if there are no metrics to flush
		// This is important to prevent leaking requests when analytics is disabled
		return nil
	}

	json, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	iostream := iostreams.FromContext(ctx)

	if iostream.IsInteractive() {
		flyctl, err := os.Executable()
		if err != nil {
			return err
		}

		cmd := exec.Command(flyctl, "metrics", "send")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		go func() {
			stdin.Write(json)
			stdin.Close()
		}()

		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "FLY_NO_UPDATE_CHECK=1")

		agent.SetSysProcAttributes(cmd)

		if err := cmd.Start(); err != nil {
			return err
		}

		if err := cmd.Process.Release(); err != nil {
			return err
		}
	} else {
		// Don't check for errors in non-interactive mode
		// because we don't want to impact other operations
		// if metrics sending fails.
		SendMetrics(ctx, string(json))
	}

	return nil
}

func SendMetrics(ctx context.Context, jsonData string) error {
	cfg := config.FromContext(ctx)
	metricsToken, err := GetMetricsToken(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Metrics token unavailable: %v\n", err)
		return nil
	}

	baseURL := cfg.MetricsBaseURL
	endpoint := baseURL + "/metrics_post"
	userAgent := fmt.Sprintf("flyctl/%s", buildinfo.Info().Version)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = sendMetricsRequest(timeoutCtx, endpoint, metricsToken, userAgent, []byte(jsonData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Metrics send issue: %v\n", err)
	}

	return nil
}

func sendMetricsRequest(ctx context.Context, endpoint, token, userAgent string, data []byte) error {
	request, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", userAgent)

	client := createHTTPClient()

	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("failed to send metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metrics send failed with status %d", resp.StatusCode)
	}

	return nil
}

func createHTTPClient() *http.Client {
	retryTransport := rehttp.NewTransport(
		http.DefaultTransport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(3),
			rehttp.RetryTimeoutErr(),
		),
		rehttp.ConstDelay(0),
	)

	return &http.Client{
		Transport: retryTransport,
		Timeout:   time.Second * 5,
	}
}
