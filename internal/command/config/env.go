package config

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newEnv() (cmd *cobra.Command) {
	const (
		short = "Display an app's runtime environment variables"
		long  = `Display an app's runtime environment variables. It displays a section for
secrets and another for config file defined environment variables.`
	)
	cmd = command.New("env", short, long, runEnv,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd, flag.App(), flag.AppConfig())
	return
}

func runEnv(ctx context.Context) error {
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)
	io := iostreams.FromContext(ctx)

	secrets, err := apiClient.GetAppSecrets(ctx, appName)
	if err != nil {
		return err
	}

	secretRows := lo.Map(secrets, func(s api.Secret, _ int) []string {
		return []string{s.Name, s.Digest, s.CreatedAt.Format("2006-01-02T15:04:05")}
	})
	if err := render.Table(io.Out, "Secrets", secretRows, "Name", "Digest", "Created At"); err != nil {
		return err
	}

	cfg, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	if cfg.ForMachines() {
		envRows := lo.Map(lo.Entries(cfg.Env), func(e lo.Entry[string, string], _ int) []string {
			return []string{e.Key, e.Value}
		})
		return render.Table(io.Out, "Environment Variables", envRows, "Name", "Value")
	} else {
		vars, ok := cfg.RawDefinition["env"].(map[string]any)
		if !ok {
			return nil
		}

		envRows := lo.Map(lo.Entries(vars), func(e lo.Entry[string, any], _ int) []string {
			return []string{e.Key, fmt.Sprintf("%s", e.Value)}
		})

		return render.Table(io.Out, "Environment Variables", envRows, "Name", "Value")
	}
}
