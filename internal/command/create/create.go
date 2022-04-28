package create

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
)

// TODO: deprecate & remove
func New() (cmd *cobra.Command) {
	const (
		long = `The CREATE command will both register a new application
with the Fly platform and create the fly.toml file which controls how
the application will be deployed. The --builder flag allows a cloud native
buildpack to be specified which will be used instead of a Dockerfile to
create the application image when it is deployed.
`
		short = `Create a new application`
		usage = "create [APPNAME]"
	)

	cmd = command.New(usage, short, long, apps.RunCreate,
		command.RequireSession)

	cmd.Args = cobra.RangeArgs(0, 1)

	// TODO: the -name & generate-name flags should be deprecated

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Description: "The app name to use",
		},
		flag.Bool{
			Name:        "generate-name",
			Description: "Generate a name for the app",
		},
		flag.String{
			Name:        "network",
			Description: "Specify custom network id",
		},
		flag.Org(),
	)

	return cmd
}
