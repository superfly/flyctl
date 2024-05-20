package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
)

func newCreate() (cmd *cobra.Command) {
	const (
		long = `Create a new application on the Fly platform.
This command won't generate a fly.toml configuration file, but you can
fetch one with 'fly config save -a <app_name>'.`

		short = "Create a new application."
		usage = "create <app name>"
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
			Hidden:      true,
		},
		flag.Org(),
	)

	flag.Add(cmd, flag.JSONOutput())
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
		apiClient     = flyutil.ClientFromContext(ctx)
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

	input := fly.CreateAppInput{
		Name:           name,
		OrganizationID: org.ID,
		Machines:       true,
	}

	if v := flag.GetString(ctx, "network"); v != "" {
		input.Network = fly.StringPointer(v)
	}

	app, err := apiClient.CreateApp(ctx, input)
	if err != nil {
		return err
	}

	f, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{AppName: app.Name})
	if err != nil {
		return err
	} else if err := f.WaitForApp(ctx, app.Name); err != nil {
		return err
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, app)
	}

	fmt.Fprintf(io.Out, "New app created: %s\n", app.Name)
	return nil
}
