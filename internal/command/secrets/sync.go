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
		long = `Make the next ` + "`fly deploy`" + ` from this computer use the app's latest
secrets, including ones set elsewhere, such as in the dashboard or from
another computer.

fly remembers the version of the secrets it last set for each app. Run this
command if secrets were changed elsewhere and a deploy from here doesn't
pick them up. It doesn't change any secrets and doesn't restart Machines.`
		short = "Sync local state with the latest app secrets"
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
	flapsClient := flapsutil.ClientFromContext(ctx)

	if err := appsecrets.Sync(ctx, flapsClient, appName); err != nil {
		return fmt.Errorf("sync secrets: %w", err)
	}

	return nil
}
