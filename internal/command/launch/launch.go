package launch

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/gql"

	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image.`
		short = long
	)

	cmd = command.New("maunch", short, long, run, command.RequireSession)

	cmd.Args = cobra.NoArgs
	cmd.Hidden = true

	flag.Add(cmd,
		flag.Region(),
		flag.Image(),
		flag.Now(),
		flag.RemoteOnly(),
		flag.LocalOnly(),
		flag.BuildOnly(),
		flag.Push(),
		flag.Org(),
		flag.Dockerfile(),
		flag.Bool{
			Name:        "no-deploy",
			Description: "Do not prompt for deployment",
		},
	)

	return
}

func run(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	gqlClient := client.GenqClient

	// MVP: Launch a single machine in the nearest region, from a Docker image, into a fresh app, with the standard vm size
	// 1. Prompt if image runs a web service. If so, generate a services section for 'fly.toml'
	// [http_service]
	// internal_port = 8080
	// force_https = true
	//
	// 2. Create app via Flaps
	// 3. Detect nearest region
	// 4. Launch machine in detected region with

	var nearestRegionCode string

	if resp, err := gql.GetNearestRegion(ctx, *gqlClient); err != nil {
		return err
	} else {
		nearestRegionCode = resp.NearestRegion.Code
	}

	org, err := prompt.Org(ctx, nil)

	if err != nil {
		return
	}

	// Launch a remote builder when we build from source
	// go gql.EnsureRemoteBuilder(ctx, gqlClient, org.ID)

	var appName string

	if appName, err = apps.SelectAppName(ctx); err != nil {
		return
	}

	resp, err := gql.CreateApp(ctx, *gqlClient, appName, org.ID)

	if err != nil {
		return
	}

	mApp := resp.CreateApp.App

	fmt.Fprintf(io.Out, "Created app %s in org %s\n", mApp.Name, org.Slug)

	appConfig := app.NewConfig()
	appConfig.AppName = mApp.Name

	appConfig.WriteToDisk()

	fmt.Fprintf(io.Out, "Wrote to fly.toml\n")

	appCompact := &api.AppCompact{
		Name:         mApp.Name,
		Organization: *org,
	}

	flapsClient, err := flaps.New(ctx, appCompact)

	machineConfig := &api.MachineConfig{
		Image: flag.GetString(ctx, "image"),
		Services: []interface{}{
			map[string]interface{}{
				"protocol":      "tcp",
				"internal_port": 80,
				"ports": []map[string]interface{}{
					{
						"port":     443,
						"handlers": []string{"http", "tls"},
					},
				},
			},
		},
	}

	launchInput := api.LaunchMachineInput{
		AppID:   mApp.Name,
		OrgSlug: org.ID,
		Region:  nearestRegionCode,
		Config:  machineConfig,
	}

	_, err = flapsClient.Launch(ctx, launchInput)

	if err != nil {
		return err
	}

	return
}
