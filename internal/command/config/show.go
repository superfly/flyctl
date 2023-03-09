package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newShow() (cmd *cobra.Command) {
	const (
		short = "Show an app's configuration"
		long  = `Show an application's configuration. The configuration is presented
in JSON format. The configuration data is retrieved from the Fly service.`
	)
	cmd = command.New("show", short, long, runShow,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	cmd.Aliases = []string{"display"}
	flag.Add(cmd, flag.App(), flag.AppConfig())
	return
}

func runShow(ctx context.Context) error {
	var (
		err       error
		appName   = appconfig.NameFromContext(ctx)
		apiClient = client.FromContext(ctx).API()
	)
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("error getting app with name %s: %w", appName, err)
	}
	ctx, err = apps.BuildContext(ctx, appCompact)
	if err != nil {
		return err
	}
	cfg, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	buf, _ := json.MarshalIndent(cfg, "", "    ")
	fmt.Fprintln(out, string(buf))
	return nil
}
