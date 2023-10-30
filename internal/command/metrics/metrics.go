package metrics

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/metrics"
)

func New() (cmd *cobra.Command) {
	// TODO: hide this command
	const (
		short = "Commands that handle sending any metrics to flyctl-metrics"
		long  = short + "\n"
		usage = "metrics <command>"
	)

	cmd = command.New(usage, short, long, nil)

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

	cmd = command.New("send", short, long, run)
	cmd.Hidden = true
	cmd.Args = cobra.NoArgs

	return
}

func run(ctx context.Context) error {
	stdin_bytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	stdin := string(stdin_bytes)

	authToken, err := metrics.GetMetricsToken(ctx)
	if err != nil {
		return err
	}

	cfg := config.FromContext(ctx)
	request, err := http.NewRequest("POST", cfg.MetricsBaseURL+"/metrics_post", bytes.NewBuffer([]byte(stdin)))
	if err != nil {
		return err
	}

	request.Header.Set("Authorization", authToken)
	request.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Info().Version))

	client := &http.Client{
		Timeout: time.Second * 60,
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	return resp.Body.Close()
}
