package flaps_api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/watch"
)

var sharedFlags = flag.Set{
	flag.App(),
	flag.AppConfig(),
	flag.Detach(),
}

func newPost() *cobra.Command {
	const (
		short = "Run a machine from a JSON configuration file"
		long  = short + "\n"

		usage = "post <json_file> [command]"
	)

	cmd := command.New(usage, short, long, runPost,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		flag.Region(),
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "Machine name, will be generated if missing",
		},
		flag.String{
			Name:        "org",
			Description: `The organization that will own the app`,
		},
		sharedFlags,
	)

	cmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

// Does not take into account any command line flags, such as --app or --name
func loadMachineJson(ctx context.Context, path string, input *api.LaunchMachineInput) (err error) {

	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(filepath.Join(state.WorkingDirectory(ctx), path))
		if err != nil {
			return err
		}
		path = absPath
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open config file: %w", err)
	}

	defer func() {
		if e := file.Close(); err == nil {
			err = e
		}
	}()

	if err = json.NewDecoder(file).Decode(input); err != nil {

		return fmt.Errorf("failed parsing machine configuration: %w", err)
	}

	return nil
}

func runPost(ctx context.Context) error {
	var (
		appName  = app.NameFromContext(ctx)
		client   = client.FromContext(ctx).API()
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		err      error
		app      *api.AppCompact
		input    api.LaunchMachineInput
	)

	if err = loadMachineJson(ctx, flag.FirstArg(ctx), &input); err != nil {
		return err
	}

	if appName == "" {
		if input.AppID != "" {
			appName = input.AppID
		} else {
			return fmt.Errorf("specify --app or fly.toml, or use 'flyctl apps create' to create a new app")
		}
	}

	app, err = client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make API client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	if app.PlatformVersion == "nomad" {
		return fmt.Errorf("the app %s uses an earlier version of the platform that does not support machines", app.Name)
	}

	input.AppID = app.Name
	// Support a few command-line overrides, but this shouldn't be a clone of `machine run`
	if name := flag.GetString(ctx, "name"); name != "" {
		input.Name = name
	}
	if region := flag.GetString(ctx, "region"); region != "" {
		input.Region = region
	}

	// now actually launch the machine
	machine, err := flapsClient.Launch(ctx, input)
	if err != nil {
		return fmt.Errorf("could not launch machine: %w", err)
	}

	id, instanceID, state, privateIP := machine.ID, machine.InstanceID, machine.State, machine.PrivateIP

	fmt.Fprintf(io.Out, "Success! A machine has been successfully launched in app %s, waiting for it to be started\n", app.Name)
	fmt.Fprintf(io.Out, " Machine ID: %s\n", id)
	fmt.Fprintf(io.Out, " Instance ID: %s\n", instanceID)
	fmt.Fprintf(io.Out, " State: %s\n", state)

	// wait for machine to be started
	if err := mach.WaitForStartOrStop(ctx, machine, "start", time.Minute*5); err != nil {
		return err
	}

	if !flag.GetDetach(ctx) {
		fmt.Fprintln(io.Out, colorize.Green("==> Monitoring health checks"))

		if err := watch.MachinesChecks(ctx, []*api.Machine{machine}); err != nil {
			return err
		}
		fmt.Fprintln(io.Out)
	}

	fmt.Fprintf(io.Out, "Machine started, you can connect via the following private ip\n")
	fmt.Fprintf(io.Out, "  %s\n", privateIP)

	return nil
}
