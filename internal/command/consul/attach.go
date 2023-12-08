package consul

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
)

const (
	consulUrlDefaultVariableName = "FLY_CONSUL_URL"
)

func newAttach() *cobra.Command {
	const (
		short = "Attach Consul cluster to an app"
		long  = "Attach Consul cluster to an app, and setting the " + consulUrlDefaultVariableName + " secret"
		usage = "attach"
	)
	cmd := command.New(usage, short, long, runAttach,
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
			Description: "The environment variable name that will be added to the consuming app.",
		},
	)
	return cmd
}

func runAttach(ctx context.Context) error {
	var (
		apiClient  = client.FromContext(ctx).API()
		cfg        = appconfig.ConfigFromContext(ctx)
		appName    = appconfig.NameFromContext(ctx)
		secretName = flag.GetString(ctx, "variable-name")
	)
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	region := cfg.PrimaryRegion
	if region == "" {
		region = cfg.Env["PRIMARY_REGION"]
	}
	if region == "" {
		if asc, err := apiClient.AppAutoscalingConfig(ctx, appName); err == nil && len(asc.Regions) > 0 {
			region = asc.Regions[0].Code
		}
	}

	consulPayload, err := apiClient.EnablePostgresConsul(ctx, appName, region)
	if err != nil {
		return nil
	}
	secretsToSet := map[string]string{
		secretName: consulPayload.ConsulURL,
	}
	err = secrets.SetSecretsAndDeploy(ctx, appCompact, secretsToSet, false, false)
	return err
}
