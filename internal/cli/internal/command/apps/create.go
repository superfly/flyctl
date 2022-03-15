package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
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
		if name, err = selectAppName(ctx); err != nil {
			return
		}
	}

	org, err := prompt.Org(ctx, nil)
	if err != nil {
		return
	}

	input := api.CreateAppInput{
		Name:           name,
		Runtime:        "FIRECRACKER",
		OrganizationID: org.ID,
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

func selectAppName(ctx context.Context) (name string, err error) {
	const msg = "App Name:"

	if err = prompt.String(ctx, &name, msg, "", false); prompt.IsNonInteractive(err) {
		err = prompt.NonInteractiveError("name argument or flag must be specified when not running interactively")
	}

	return
}
