package config

import (
	"context"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
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
	apiClient := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	io := iostreams.FromContext(ctx)

	secrets, err := apiClient.GetAppSecrets(ctx, appName)
	if err != nil {
		return err
	}

	secretRows := lo.Map(secrets, func(s fly.Secret, _ int) []string {
		return []string{s.Name, s.Digest, s.CreatedAt.Format("2006-01-02T15:04:05")}
	})
	if err := render.Table(io.Out, "Secrets", secretRows, "Name", "Digest", "Created At"); err != nil {
		return err
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	cfg, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	envRows := lo.Map(lo.Entries(cfg.Env), func(e lo.Entry[string, string], _ int) []string {
		return []string{e.Key, e.Value}
	})
	return render.Table(io.Out, "Environment Variables", envRows, "Name", "Value")
}
