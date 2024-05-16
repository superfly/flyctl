package consul

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
)

func newDetach() *cobra.Command {
	const (
		short = "Detach Consul cluster from an app"
		long  = "Detach Consul cluster from an app, and unsetting the " + consulUrlDefaultVariableName + " secret"
		usage = "detach"
	)
	cmd := command.New(usage, short, long, runDetach,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "variable-name",
			Default:     consulUrlDefaultVariableName,
			Description: "The secret name that will be removed from the app.",
		},
	)
	return cmd
}

func runDetach(ctx context.Context) error {
	var (
		apiClient  = flyutil.ClientFromContext(ctx)
		appName    = appconfig.NameFromContext(ctx)
		secretName = flag.GetString(ctx, "variable-name")
	)
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}
	secretsToUnset := []string{secretName}
	err = secrets.UnsetSecretsAndDeploy(ctx, appCompact, secretsToUnset, false, false)
	return err
}
