package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
		SendMetrics(ctx, string(json))
	}

	return nil
}

// / Spens up to 15 seconds sending all metrics collected so far to flyctl-metrics post endpoint
func SendMetrics(ctx context.Context, json string) error {
	fmt.Fprintf(os.Stderr, "Non-interactive mode, sending metrics\n")

	cfg := config.FromContext(ctx)

	// Try to get the token first from the original context
	metricsToken, err := GetMetricsToken(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get metrics token from original context: %v\n", err)
		return err
	}

	baseURL := cfg.MetricsBaseURL
	userAgent := fmt.Sprintf("flyctl/%s", buildinfo.Info().Version)

	fmt.Fprintf(os.Stderr, "Using metrics endpoint: %s\n", baseURL)
	fmt.Fprintf(os.Stderr, "Has metrics token: %v\n", metricsToken != "")

	errChan := make(chan error, 1)

	go func(url, token, agent string, data []byte) {
		request, err := http.NewRequest("POST", url+"/metrics_post", bytes.NewBuffer(data))
		if err != nil {
			errChan <- err
			return
		}

		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("User-Agent", agent)

		retryTransport := rehttp.NewTransport(
			http.DefaultTransport,
			rehttp.RetryAll(
				rehttp.RetryMaxRetries(3),
				rehttp.RetryTimeoutErr(),
			),
			rehttp.ConstDelay(0),
		)

		client := http.Client{
			Transport: retryTransport,
			Timeout:   time.Second * 5,
		}

		resp, err := client.Do(request)
		if err != nil {
			errChan <- fmt.Errorf("failed to send metrics: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errChan <- fmt.Errorf("metrics send failed with status %d: %s", resp.StatusCode, string(body))
			return
		}

		// Drain the body to reuse connections
		_, err = io.Copy(io.Discard, resp.Body)
		if err != nil {
			errChan <- fmt.Errorf("failed to read response body: %w", err)
			return
		}

		errChan <- nil
	}(baseURL, metricsToken, userAgent, []byte(json))

	select {
	case err := <-errChan:
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send metrics: %v\n", err)
			return err
		}
		fmt.Fprintf(os.Stderr, "Metrics send completed successfully\n")
	case <-time.After(15 * time.Second):
		fmt.Fprintf(os.Stderr, "Metrics send timed out after 15 seconds\n")
		return fmt.Errorf("metrics send timed out")
	}

	return nil
}
