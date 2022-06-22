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

	cmd = command.New("maunch", short, long, run, command.RequireSession, command.LoadAppConfigIfPresent)

	cmd.Args = cobra.NoArgs
	cmd.Hidden = true

	flag.Add(cmd,
		flag.Region(),
		flag.Image(),
		flag.Now(),
		flag.RemoteOnly(true),
		flag.LocalOnly(),
		flag.BuildOnly(),
		flag.Push(),
		flag.Org(),
		flag.Dockerfile(),
		flag.Bool{
			Name:        "no-deploy",
			Description: "Do not prompt for deployment",
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting",
		},
		flag.Bool{
			Name:        "generate-name",
			Description: "Always generate a name for the app",
		},
	)

	return
}

func run(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	gqlClient := client.GenqClient

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

	image := flag.GetString(ctx, "image")

	appConfig := app.NewConfig()
	appConfig.AppName = mApp.Name
	appConfig.Build = &app.Build{
		Image: image,
	}

	httpService, err := prompt.Confirm(ctx, "Does this app run an http service?")

	if err != nil {
		return
	}

	if httpService {
		appConfig.HttpService = new(app.HttpService)
		appConfig.HttpService.ForceHttps = true
		appConfig.HttpService.InternalPort = 8080
	}

	appConfig.WriteToDisk()

	fmt.Fprintf(io.Out, "Wrote to fly.toml\n")

	return deploy(ctx, appConfig)
}

func deploy(ctx context.Context, config *app.Config) (err error) {

	client := client.FromContext(ctx).API()

	region, err := client.GetNearestRegion(ctx)

	if err != nil {
		return
	}

	app, err := client.GetAppCompact(ctx, config.AppName)

	if err != nil {
		return
	}

	flapsClient, err := flaps.New(ctx, app)

	if err != nil {
		return
	}

	machineConfig := &api.MachineConfig{
		Image: config.Build.Image,
	}

	if config.HttpService != nil {
		machineConfig.Services = []interface{}{
			map[string]interface{}{
				"protocol":      "tcp",
				"internal_port": config.HttpService.InternalPort,
				"ports": []map[string]interface{}{
					{
						"port":     443,
						"handlers": []string{"http", "tls"},
					},
					{
						"port":        80,
						"handlers":    []string{"http"},
						"force_https": config.HttpService.ForceHttps,
					},
				},
			},
		}
	}
	err = config.Validate()

	if err != nil {
		return err
	}

	launchInput := api.LaunchMachineInput{
		AppID:   config.AppName,
		OrgSlug: app.Organization.ID,
		Region:  region.Code,
		Config:  machineConfig,
	}

	_, err = flapsClient.Launch(ctx, launchInput)

	return
}
