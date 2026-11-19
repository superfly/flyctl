package config

import (
	"context"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newEnv() (cmd *cobra.Command) {
	const (
		short = "Display an app's runtime environment variables"
		long  = `Show the app's secrets and environment variables, in two tables. Secrets are
listed by name and digest only, never by value. Environment variables come
from the [env] section of the app's deployed configuration, not your local
fly.toml. The app comes from --app or fly.toml. Manage secrets with
` + "`fly secrets`" + `.`
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
	appName := appconfig.NameFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	io := iostreams.FromContext(ctx)

	secrets, err := appsecrets.List(ctx, flapsClient, appName)
	if err != nil {
		return err
	}

	secretRows := lo.Map(secrets, func(s fly.AppSecret, _ int) []string {
		return []string{s.Name, s.Digest}
	})
	if err := render.Table(io.Out, "Secrets", secretRows, "Name", "Digest"); err != nil {
		return err
	}

	cfg, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	envRows := lo.Map(lo.Entries(cfg.Env), func(e lo.Entry[string, string], _ int) []string {
		return []string{e.Key, e.Value}
	})

	return render.Table(io.Out, "Environment Variables", envRows, "Name", "Value")
}
