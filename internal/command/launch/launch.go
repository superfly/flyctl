package launch

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func New() (cmd *cobra.Command) {
	const (
		long  = `Create and configure a new app from source code or a Docker image.`
		short = long
	)

	cmd = command.New("maunch", short, long, run, command.RequireSession)

	cmd.Args = cobra.NoArgs

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

func run(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	var io := iostreams.FromContext(ctx)

	org := flag.GetString(ctx, "org")

	if org == "" {
		org = "personal"
	}

	go imgsrc.EagerlyEnsureRemoteBuilder(ctx, client, org)

	// MVP: Launch a single machine in the nearest region, from a Docker image, into a fresh app, with the standard vm size
	// 1. Prompt if image runs a web service. If so, generate a services section for 'fly.toml'
	// [http_service]
	// internal_port = 8080
	// force_https = true
	//
	// 2. Create app via Flaps
	// 3. Detect nearest region
	// 4. Launch machine in detected region with
	org, err := selectOrganization(ctx, client, orgSlug, nil)

	name, err := apps.SelectAppName(ctx)
	flaps.CreateApp(name, org)
	fmt.Fprintf(io.Out, "Created app %s", name)
	return
}
