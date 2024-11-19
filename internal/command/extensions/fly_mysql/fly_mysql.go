package fly_mysql

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage MySQL database clusters"
		long  = short + "\n"
	)

	cmd = command.New("mysql", short, long, nil)
	cmd.AddCommand(create(), list(), status(), destroy(), update())

	return cmd
}

var SharedFlags = flag.Set{
	flag.Int{
		Name:        "size",
		Description: "The number of members in your cluster",
	},
	flag.Int{
		Name:        "cpu",
		Description: "The number of CPUs assigned to each cluster member",
	},
	flag.Int{
		Name:        "memory",
		Description: "Memory (in GB) assigned to each cluster member",
	},
}

func optionsFromFlags(ctx context.Context, options map[string]interface{}) map[string]interface{} {

	if options == nil {
		options = gql.AddOnOptions{}
	}

	flags := []string{"size", "cpu", "memory", "disk"}

	for _, f := range flags {
		if flag.IsSpecified(ctx, f) {
			if val := flag.GetInt(ctx, f); val != 0 {
				options[f] = val
			}
		}
	}

	return options
}
