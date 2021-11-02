package apps

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/client"
)

func newCreate() *cobra.Command {
	create := command.FromDocstrings("apps.create", runCreate,
		command.RequireOrg,
	)

	create.Args = cobra.RangeArgs(0, 1)

	flag.Add(create,
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

	return create
}

func runCreate(ctx context.Context) (err error) {
	var (
		io            = iostreams.FromContext(ctx)
		aName         = flag.FirstArg(ctx)
		fName         = flag.GetString(ctx, "name")
		fGenerateName = flag.GetBool(ctx, "generate-name")
	)

	var name string
	switch {
	case aName != "" && fName != "" && aName != fName:
		err = fmt.Errorf("two app names specified %s and %s, only one may be specified",
			aName, fName)

		return
	case aName != "":
		name = aName
	case fName != "":
		name = fName
	default:
		if name, err = selectAppName(ctx, fGenerateName); err != nil {
			return
		}
	}

	input := api.CreateAppInput{
		Name:           name,
		Runtime:        "FIRECRACKER",
		OrganizationID: state.Org(ctx).ID,
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
		fmt.Fprintf(io.Out, "New app created: %s\n", app.Name)
	}

	return err
}

func selectAppName(ctx context.Context, autoGenerate bool) (name string, err error) {
	msg := "App Name"
	if autoGenerate {
		msg += " (leave blank to use an auto-generated name)"
	}
	msg += ":"

	if err = prompt.String(ctx, &name, msg, ""); prompt.IsNonInteractive(err) {
		err = errors.New("name argument or flag must be specified when not running interactively")
	}

	return
}
