package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
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
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	cfg, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(io.Out, string(b))
	return nil
}
