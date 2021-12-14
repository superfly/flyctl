package launch

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

// TODO: deprecate & remove
func New() *cobra.Command {
	const (
		long  = `Create and configure a Fly application from source code or a Docker image reference.`
		short = `Create and configure a Fly application from source code or a Docker image reference.`
	)

	launch := command.New("launch", short, long, RunLaunch, command.RequireSession)

	flag.Add(launch,
		flag.Org(),
		flag.AppName(),
		flag.Region(),
		flag.Image(),
		flag.Now(),
		flag.NoDeploy(),
		flag.GenerateName(),
		flag.RemoteOnly(),
	)

	flag.Add(launch,
		flag.String{
			Name:        "dockerfile",
			Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
		},
		flag.Bool{
			Name:        "copy-config",
			Description: "Use the configuration file if present without prompting.",
			Default:     false,
		},
	)

	return launch
}

func RunLaunch(ctx context.Context) (err error) {
	return
}
