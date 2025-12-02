package metrics

import (
	"context"
	"fmt"
	"io"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Manage app metrics"
		long  = short + "\n"
		usage = "metrics <command>"
	)

	cmd = command.New(usage, short, long, nil)

	cmd.AddCommand(
		newSend(),
		newShow(),
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

	return metrics.SendMetrics(ctx, stdin_value)
}

func newShow() (cmd *cobra.Command) {
	const (
		short = "Open browser to app's Grafana metrics dashboard"
		long  = short + "\n"
	)

	cmd = command.New("show", short, long, runShow,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
	)

	return
}

func runShow(ctx context.Context) error {
	iostream := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	client := flyutil.ClientFromContext(ctx)

	app, err := client.GetAppBasic(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get app info: %w", err)
	}

	url := fmt.Sprintf("https://fly-metrics.net/d/fly-app/fly-app?orgId=%s&var-app=%s", app.Organization.InternalNumericID, appName)

	fmt.Fprintf(iostream.Out, "Opening %s\n", url)
	return open.Run(url)
}
