package secrets

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newList() (cmd *cobra.Command) {
	const (
		long = `List the secrets available to the application. It shows each secret's
name, a digest of its value and the time the secret was last set. The
actual value of the secret is only available to the application.`
		short = `List application secret names, digests and creation times`
		usage = "list [flags]"
	)

	cmd = command.New(usage, short, long, runList, command.RequireSession, command.RequireAppName)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	client := flyutil.ClientFromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	ctx, err = setFlapsClient(ctx, app)
	if err != nil {
		return err
	}

	flapsClient := flapsutil.ClientFromContext(ctx)

	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	minVers, err := appsecrets.GetAppSecretsMinvers(app.ID)
	if err != nil {
		return err
	}

	secrets, err := flapsClient.ListAppSecrets(ctx, minVers, false)
	if err != nil {
		return err
	}

	var rows [][]string

	for _, secret := range secrets {
		rows = append(rows, []string{
			secret.Name,
			secret.Digest,
		})
	}

	headers := []string{
		"Name",
		"Digest",
	}
	if cfg.JSONOutput {
		return render.JSON(out, secrets)
	} else {
		return render.Table(out, "", rows, headers...)
	}
}
