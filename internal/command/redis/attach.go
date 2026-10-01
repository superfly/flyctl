package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func AttachDatabase(ctx context.Context, db *gql.AddOn, appName string) (err error) {
	return attachDatabase(ctx, db.Name, db.PublicUrl, appName)
}

func attachDatabase(ctx context.Context, dbName, publicUrl, appName string) (err error) {
	if publicUrl == "" {
		return fmt.Errorf("redis database %s has no public URL yet; it may still be provisioning", dbName)
	}

	io := iostreams.FromContext(ctx)
	s := map[string]string{"REDIS_URL": publicUrl}

	flapsClient := flapsutil.ClientFromContext(ctx)
	err = appsecrets.Update(ctx, flapsClient, appName, s, nil)
	if err != nil {
		fmt.Fprintf(io.Out, "\nCould not attach Redis database %s to app %s\n", dbName, appName)
	} else {
		fmt.Fprintf(io.Out, "\nRedis database %s is set on %s as the REDIS_URL environment variable\n", dbName, appName)
	}

	return err
}

func newAttach() (cmd *cobra.Command) {
	const (
		short = "Attach a Redis database to an app, setting REDIS_URL"
		long  = short + "\n"
		usage = "attach <name>"
	)

	cmd = command.New(usage, short, long, runAttach,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runAttach(ctx context.Context) (err error) {
	var (
		name    = flag.FirstArg(ctx)
		appName = appconfig.NameFromContext(ctx)
		client  = flyutil.ClientFromContext(ctx).GenqClient()
	)

	response, err := gql.GetAddOn(ctx, client, name, string(gql.AddOnTypeUpstashRedis))
	if err != nil {
		return err
	}

	addOn := response.AddOn

	return attachDatabase(ctx, addOn.Name, addOn.PublicUrl, appName)
}
