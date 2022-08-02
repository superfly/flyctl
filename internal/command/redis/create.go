package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newCreate() (cmd *cobra.Command) {
	const (
		long = `Create a new Redis instance`

		short = long
		usage = "create"
	)

	cmd = command.New(usage, short, long, runCreate, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		out = iostreams.FromContext(ctx).Out
		//client = client.FromContext(ctx).API()
	)

	org, err := prompt.Org(ctx)

	if err != nil {
		return err
	}

	service, err := ProvisionRedis(ctx, org, "us-east-1")

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Redis instance is available to apps in the %s organization at %s\n", org.Slug, service.PublicUrl)

	return
}

func ProvisionRedis(ctx context.Context, org *api.Organization, region string) (service *api.AddOn, err error) {
	client := client.FromContext(ctx).API()
	service, err = client.ProvisionService(ctx, "upstash_redis", org.ID, region)

	if err != nil {
		return
	}

	return service, nil
	// client.SetSecrets(ctx, app.Name, secrets)
	// fmt.Fprintln(out, "Launching Redis instance...")
	// machine, err := flapsClient.Launch(ctx, launchInput)

}
