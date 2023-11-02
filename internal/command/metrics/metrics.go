package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Commands that handle sending any metrics to flyctl-metrics"
		long  = short + "\n"
		usage = "metrics <command>"
	)

	cmd = command.New(usage, short, long, nil)
	cmd.Hidden = true

	cmd.AddCommand(
		newSend(),
	)

	return
}

func newSend() (cmd *cobra.Command) {
	const (
		short = "Send any metrics in stdin to flyctl-metrics"
		long  = short + "\n"
	)

	cmd = command.New("send", short, long, run, func(ctx context.Context) (context.Context, error) {
		return metrics.WithDisableFlushMetrics(ctx), nil
	})
	cmd.Hidden = true
	cmd.Args = cobra.NoArgs

	return
}

func run(ctx context.Context) error {
	iostream := iostreams.FromContext(ctx)
	stdin := iostream.In

	stdin_bytes, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}

	stdin_value := string(stdin_bytes)

	authToken, err := metrics.GetMetricsToken(ctx)
	if err != nil {
		return err
	}

	cfg := config.FromContext(ctx)
	request, err := http.NewRequest("POST", cfg.MetricsBaseURL+"/metrics_post", bytes.NewBuffer([]byte(stdin_value)))
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", authToken)
	request.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Info().Version))

	retryTransport := rehttp.NewTransport(http.DefaultTransport, rehttp.RetryAll(rehttp.RetryMaxRetries(3), rehttp.RetryTimeoutErr()), rehttp.ConstDelay(time.Second))

	client := http.Client{
		Transport: retryTransport,
		Timeout:   time.Second * 30,
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	return resp.Body.Close()
}
