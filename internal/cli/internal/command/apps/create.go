package apps

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
)

func newCreate() *cobra.Command {
	create := command.FromDocstrings("apps.create", runCreate,
		command.RequireOrg,
	)

	create.Args = cobra.RangeArgs(0, 1)

	flag.Add(create,
		flag.Org(),
		flag.String{
			Name:        "network",
			Description: "Specify custom network id",
		},
	)

	return create
}

func runCreate(ctx context.Context) error {
	var (
		name = flag.FirstArg(ctx)
		org  = flag.GetOrg(ctx)
	)

	input := api.CreateAppInput{
		Name:           name,
		Runtime:        "FIRECRACKER",
		OrganizationID: org,
	}

	// set network if flag is set
	if v := flag.GetString(ctx, "network"); v != "" {
		input.Network = api.StringPointer(v)
	}

	// The creation magic happens here....
	app, err := client.FromContext(ctx).
		API().
		CreateApp(ctx, input)

	if err == nil {
		fmt.Fprintf(os.Stdout, "New app created: %s\n", app.Name)
	}

	return err
}
