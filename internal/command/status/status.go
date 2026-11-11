// Package status implements the status command chain.
package status

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/azazeal/pause"
	"github.com/inancgumus/screen"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Show the app's name, organization, hostname and image, followed by a table
of its Machines with each one's process group, ID, version, region, state,
role, health checks and last update time. When a newer version of a
Fly.io-tracked image is available, it also lists the Machines that can
update (see ` + "`fly image update`" + `). The app comes from --app or fly.toml.

--watch redraws the status every --rate seconds (5 by default) until you
press Ctrl-C; it needs a terminal and can't be combined with --json. --json
prints the app and its Machines as JSON. For logs, run ` + "`fly logs`" + `.`
		short = "Show app status"
	)

	cmd = command.New("status", short, long, run,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		flag.Bool{
			Name:        "all",
			Description: "Show completed instances",
		},
		flag.Bool{
			Name:        "deployment",
			Description: "Always show deployment status",
		},
		flag.Bool{
			Name:        "watch",
			Description: "Refresh details",
		},
		flag.Int{
			Name:        "rate",
			Description: "Refresh Rate for --watch",
			Default:     5,
		},
	)

	return
}

func run(ctx context.Context) error {
	watch := flag.GetBool(ctx, "watch")
	if watch && config.FromContext(ctx).JSONOutput {
		return errors.New("--watch and --json are not supported together")
	}

	if !watch {
		return runOnce(ctx)
	}

	return runWatch(ctx)
}

func runOnce(ctx context.Context) error {
	return once(ctx, iostreams.FromContext(ctx).Out)
}

func once(ctx context.Context, out io.Writer) (err error) {
	var (
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get app: %w", err)
	}

	return RenderMachineStatus(ctx, app, out)
}

func runWatch(ctx context.Context) (err error) {
	streams := iostreams.FromContext(ctx)
	if !streams.IsInteractive() {
		err = errors.New("--watch is not supported for non-interactive sessions")

		return
	}
	colorize := streams.ColorScheme()

	sleep := flag.GetInt(ctx, "rate")
	if sleep < 1 || sleep > 3600 {
		err = errors.New("--rate must be in the [1, 3600] range")

		return
	}

	appName := appconfig.NameFromContext(ctx)

	var buf bytes.Buffer

	for err == nil {
		buf.Reset()

		if err = once(ctx, &buf); err != nil {
			break
		}

		header := fmt.Sprintf("%s %s %s\n\n", colorize.Bold(appName), "at:", colorize.Bold(time.Now().UTC().Format("15:04:05")))

		screen.Clear()
		screen.MoveTopLeft()

		io.Copy(streams.Out, io.MultiReader(
			strings.NewReader(header),
			&buf,
		))

		pause.For(ctx, time.Duration(sleep)*time.Second)
	}

	// Interrupted with Ctrl-C
	if errors.Is(ctx.Err(), context.Canceled) {
		err = nil
	}

	return
}
