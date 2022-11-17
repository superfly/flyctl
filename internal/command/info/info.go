package info

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		long  = `Shows information about the application on the Fly platform.`
		short = `Shows information about the application`
	)

	cmd := command.New("info", short, long, runInfo,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runInfo(ctx context.Context) error {
	var (
		client  = client.FromContext(ctx).API()
		appName = app.NameFromContext(ctx)
	)

	appInfo, err := client.GetAppInfo(ctx, appName)
	if err != nil {
		return err
	}

	if appInfo.PlatformVersion == "machines" {
		return showMachineInfo(ctx, appName)
	} else {
		return showNomadInfo(ctx, appInfo)
	}
}

// TODO - Move this into a higher level package, so it can be used elsewhere.
func formatRelativeTime(t time.Time) string {
	if t.Before(time.Now()) {
		dur := time.Since(t)
		if dur.Seconds() < 1 {
			return "just now"
		}
		if dur.Seconds() < 60 {
			return fmt.Sprintf("%ds ago", int64(dur.Seconds()))
		}
		if dur.Minutes() < 60 {
			return fmt.Sprintf("%dm%ds ago", int64(dur.Minutes()), int64(math.Mod(dur.Seconds(), 60)))
		}

		if dur.Hours() < 24 {
			return fmt.Sprintf("%dh%dm ago", int64(dur.Hours()), int64(math.Mod(dur.Minutes(), 60)))
		}
	} else {
		dur := time.Until(t)
		if dur.Seconds() < 60 {
			return fmt.Sprintf("%ds", int64(dur.Seconds()))
		}
		if dur.Minutes() < 60 {
			return fmt.Sprintf("%dm%ds", int64(dur.Minutes()), int64(math.Mod(dur.Seconds(), 60)))
		}

		if dur.Hours() < 24 {
			return fmt.Sprintf("%dh%dm", int64(dur.Hours()), int64(math.Mod(dur.Minutes(), 60)))
		}
	}

	return formatTime(t)
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}
