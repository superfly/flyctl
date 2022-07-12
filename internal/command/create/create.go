package create

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/flag"
)

// TODO: deprecate & remove
func New() (cmd *cobra.Command) {
	cmd = &cobra.Command{
		Use:        "create",
		Hidden:     true,
		Deprecated: "replaced by 'apps create'",
	}

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Description: "The app name to use",
		},
		flag.Bool{
			Name:        "generate-name",
			Description: "Generate an app name",
		},
		flag.String{
			Name:        "network",
			Description: "Specify custom network id",
		},
		flag.Bool{
			Name:        "machines",
			Description: "Use the machines platform",
		},
		flag.Org(),
	)

	return cmd
}
