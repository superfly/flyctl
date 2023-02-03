package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func newCreate() (cmd *cobra.Command) {
	const (
		long = `The APPS CREATE command will register a new application
with the Fly platform. It will not generate a configuration file, but one
may be fetched with 'fly config save -a <app_name>'`

		short = "Create a new application"
		usage = "create [APPNAME]"
	)

	cmd = command.New(usage, short, long, RunCreate,
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

// TODO: make internal once the create package is removed
func RunCreate(ctx context.Context) (err error) {
	var (
		io            = iostreams.FromContext(ctx)
		cfg           = config.FromContext(ctx)
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
	case fGenerateName:
		break
	default:
		if name, err = prompt.SelectAppName(ctx); err != nil {
			return
		}
	}

	org, err := prompt.Org(ctx)
	if err != nil {
		return
	}

	input := api.CreateAppInput{
		Name:           name,
		OrganizationID: org.ID,
		Machines:       flag.GetBool(ctx, "machines"),
	}

	if v := flag.GetString(ctx, "network"); v != "" {
		input.Network = api.StringPointer(v)
	}

	app, err := client.FromContext(ctx).
		API().
		CreateApp(ctx, input)

	if err == nil {
		if cfg.JSONOutput {
			return render.JSON(io.Out, app)
		}
		fmt.Fprintf(io.Out, "New app created: %s\n", app.Name)
	}

	return err
}
