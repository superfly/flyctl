package redis

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/helpers"
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
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		out    = iostreams.FromContext(ctx).Out
		client = client.FromContext(ctx).API()
	)

	org, err := prompt.Org(ctx)

	if err != nil {
		return
	}

	password, ip = ProvisionRedis(ctx, org)

	fmt.Fprintf(out, "Your Redis instance is available to apps in the %s organization at:\nredis://default:%s@[%s]:6379\n", app.Organization.Slug, password, machine.PrivateIP)

	return
}

func ProvisionRedis(ctx context.Context, org *api.Organization) (ip string, err error) {
	appInput := api.CreateAppInput{
		OrganizationID: org.ID,
		Machines:       true,
		AppRoleID:      "redis",
	}

	app, err := client.CreateApp(ctx, appInput)

	if err != nil {
		return
	}

	flapsClient, err := flaps.New(ctx, &api.AppCompact{
		ID:   app.ID,
		Name: app.Name,
		Organization: &api.OrganizationBasic{
			ID:   app.Organization.ID,
			Slug: app.Organization.Slug,
		},
	})

	if err != nil {
		return
	}
	imageRef, err := client.GetLatestImageTag(ctx, "flyio/redis")

	if err != nil {
		return err
	}

	launchInput := api.LaunchMachineInput{
		AppID:   app.Name,
		OrgSlug: app.Organization.ID,
		Config: &api.MachineConfig{
			Image: imageRef,
		},
	}

	password, err := helpers.RandString(32)

	if err != nil {
		return err
	}

	secrets := map[string]string{
		"REDIS_PASSWORD": password,
	}

	client.SetSecrets(ctx, app.Name, secrets)
	fmt.Fprintln(out, "Launching Redis instance...")
	machine, err := flapsClient.Launch(ctx, launchInput)

	if err != nil {
		return
	}

}
