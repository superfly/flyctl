package redis

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
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
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	org, err := prompt.Org(ctx)

	if err != nil {
		return
	}

	appInput := api.CreateAppInput{
		OrganizationID: org.ID,
		Machines:       true,
		AppRoleID:      "redis",
	}

	app, err := client.CreateApp(ctx, appInput)

	flapsClient, err := flaps.New(ctx, &api.AppCompact{
		ID:   app.ID,
		Name: app.Name,
		Organization: &api.OrganizationBasic{
			ID:   app.Organization.ID,
			Slug: app.Organization.Slug,
		},
	})

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
	client.SetSecrets(ctx, app.Name, secrets)
	flapsClient.Launch(ctx, launchInput)
	return err
}
