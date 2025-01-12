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

	// On CI, always block on metrics send. This sucks, but the alternative is not getting metrics from CI at all. There are timeouts in place to prevent this from taking more than 15 seconds

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
	authToken, err := GetMetricsToken(ctx)
	if err != nil {
		return err
	}

	cfg := config.FromContext(ctx)
	request, err := http.NewRequest("POST", cfg.MetricsBaseURL+"/metrics_post", bytes.NewBuffer([]byte(json)))
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", authToken)
	request.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Info().Version))

	retryTransport := rehttp.NewTransport(http.DefaultTransport, rehttp.RetryAll(rehttp.RetryMaxRetries(3), rehttp.RetryTimeoutErr()), rehttp.ConstDelay(0))

	client := http.Client{
		Transport: retryTransport,
		Timeout:   time.Second * 5,
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	return resp.Body.Close()
}
