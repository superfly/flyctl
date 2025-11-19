package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/state"
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
		flag.Yes(),
		flag.Bool{
			Name:        "save",
			Description: "Save the app name to the config file",
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

	flapsClient := flapsutil.ClientFromContext(ctx)
	app, err := flapsClient.CreateApp(ctx, flaps.CreateAppRequest{
		Name:    name,
		Org:     org.RawSlug,
		Network: flag.GetString(ctx, "network"),
	})
	if err != nil {
		return err
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, app)
	}

	fmt.Fprintf(io.Out, "New app created: %s\n", app.Name)

	if flag.GetBool(ctx, "save") {
		path := state.WorkingDirectory(ctx)
		configfilename, err := appconfig.ResolveConfigFileFromPath(path)
		if err != nil {
			return err
		}

		if exists, _ := appconfig.ConfigFileExistsAtPath(configfilename); exists && !flag.GetBool(ctx, "yes") {
			confirmation, err := prompt.Confirmf(ctx,
				"An existing configuration file has been found\nOverwrite file '%s'", configfilename)
			if err != nil {
				return err
			}
			if !confirmation {
				return nil
			}
		}

		cfg := appconfig.Config{
			AppName: app.Name,
		}

		err = cfg.WriteToDisk(ctx, configfilename)
		if err != nil {
			return fmt.Errorf("failed to save app name to config file: %w", err)
		}
	}

	return nil
}
