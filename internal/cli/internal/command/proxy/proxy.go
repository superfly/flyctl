package proxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/proxy"
)

func New() *cobra.Command {
	var (
		long  = strings.Trim(`Proxies connections to a fly VM through a Wireguard tunnel`, "\n")
		short = `Proxies connections to a fly VM"`
	)

	cmd := command.New("proxy <local:remote>", short, long, run,
		command.RequireSession, command.RequireAppName)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Default:     false,
			Description: "Prompt to select from available instances from the current application",
		},
	)

	return cmd
}

func run(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	args := flag.Args(ctx)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	ports := strings.Split(args[0], ":")

	proxy.Connect(ctx, ports, app, flag.GetBool(ctx, "select"))
	return nil
}
