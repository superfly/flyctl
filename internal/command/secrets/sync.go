package secrets

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
)

func newSync() (cmd *cobra.Command) {
	const (
		long  = `Sync flyctl with the latest versions of app secrets, even if they were set elsewhere`
		short = long
		usage = "sync [flags]"
	)

	cmd = command.New(usage, short, long, runSync, command.RequireSession, command.RequireAppName)

	flag.Add(cmd,
		sharedFlags,
	)

	return cmd
}

// runSync updates the app's minsecret version to the current point in time.
// Any secrets set previous to this point in time will be visible when flyctl
// deploys apps. This addresses an issue where flyctl maintains a local copy
// of the min secrets version for app secrets that it updates, but is not aware
// of the version set elsewhere, such as by the dashboard or another flyctl.
func runSync(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)
	ctx, flapsClient, app, err := flapsutil.SetClient(ctx, nil, appName)
	if err != nil {
		return err
	}

	if err := appsecrets.Sync(ctx, flapsClient, app.Name); err != nil {
		return fmt.Errorf("sync secrets: %w", err)
	}
	return nil
}
