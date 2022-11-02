package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/ssh"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newImport() *cobra.Command {
	const (
		short = "Import data from an existing database"
		long  = short + "\n"
		usage = "import"
	)

	cmd := command.New(usage, short, long, runImport,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "source",
			Shorthand:   "s",
			Description: "Source database URI",
		},
	)

	return cmd
}

func runImport(ctx context.Context) error {
	var (
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		appName  = app.NameFromContext(ctx)
		client   = client.FromContext(ctx).API()
	)
	const MinPostgresHaVersion, MinPostgresStandaloneVersion = "0.0.19", "0.0.19"

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("error getting app %s: %w", appName, err)
	}

	if !app.IsPostgresApp() {
		return fmt.Errorf("%s is not a postgres app", appName)
	}

	if app.PlatformVersion == "nomad" {
		return fmt.Errorf("import is not supported on nomad apps")
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	dialer, err := agentclient.ConnectToTunnel(ctx, app.Organization.Slug)
	if err != nil {
		return err
	}
	ctx = agent.DialerWithContext(ctx, dialer)

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("list of machines could not be retrieved: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}
	if err = hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresStandaloneVersion); err != nil {
		return err
	}

	leader, _ := machinesNodeRoles(ctx, machines)

	region := leader.Region

	pgclient := flypg.NewFromInstance(leader.PrivateIP, dialer)

	fmt.Fprintln(io.Out, "Creating temporary user on target cluster")

	user, err := helpers.RandString(4)
	if err != nil {
		return err
	}
	password, err := helpers.RandString(15)
	if err != nil {
		return err
	}

	user = fmt.Sprintf("flyctl_migrator_%s", user)

	if err = pgclient.CreateUser(ctx, user, password, true, true); err != nil {
		return fmt.Errorf("error creating user %w", err)
	}
	defer pgclient.DeleteUser(ctx, user)

	target := fmt.Sprintf("postgres://%s:%s@%s.internal:5432", user, password, app.Name)

	source := flag.GetString(ctx, "source")

	secrets := map[string]string{
		"SOURCE_DATABASE_URI": source,
		"TARGET_DATABASE_URI": target,
	}

	fmt.Fprintln(io.Out, "Setting secrets...")

	if _, err := client.SetSecrets(ctx, app.Name, secrets); err != nil {
		return fmt.Errorf("error setting secrets %w", err)
	}
	defer client.UnsetSecrets(ctx, app.Name, []string{"SOURCE_DATABASE_URI", "TARGET_DATABASE_URI"})

	fmt.Fprintln(io.Out, "Creating temporary machine")

	flapClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("error creating flap client %w", err)
	}

	var migratorIMage = "flyio/postgres-migrator:latest"

	input := api.LaunchMachineInput{
		OrgSlug: app.Organization.Slug,
		AppID:   app.ID,
		Region:  region,
		Config: &api.MachineConfig{
			Image:  migratorIMage,
			VMSize: "shared-cpu-2x",
			Metadata: map[string]string{
				"process": "postgres-migrator",
			},
			Env: map[string]string{
				"POSTGRES_PASSWORD": "pass",
			},
		},
	}

	migrator, err := flapClient.Launch(ctx, input)
	if err != nil {
		return fmt.Errorf("error launching machine %w", err)
	}
	defer flapClient.Destroy(ctx, api.RemoveMachineInput{AppID: app.ID, ID: migrator.ID, Kill: true})

	fmt.Fprintf(io.Out, "Waiting for machine to be ready %s\n", colorize.Bold(migrator.ID))

	if err = machine.WaitForStartOrStop(ctx, migrator, "start", time.Minute*5); err != nil {
		return fmt.Errorf("error waiting for machine to start %w", err)
	}

	machines = append(machines, migrator)

	// Acquire leases
	fmt.Fprintf(io.Out, "Attempting to acquire lease(s)\n")

	for _, machine := range machines {
		lease, err := flapsClient.GetLease(ctx, machine.ID, api.IntPointer(120))
		if err != nil {
			return fmt.Errorf("failed to obtain lease: %w", err)
		}
		machine.LeaseNonce = lease.Data.Nonce

		// Ensure lease is released on return
		defer flapsClient.ReleaseLease(ctx, machine.ID, machine.LeaseNonce)

		fmt.Fprintf(io.Out, "  Machine %s: %s\n", colorize.Bold(machine.ID), lease.Status)
	}

	fmt.Fprintln(io.Out, "Running database import with pgdumb ...")

	fmt.Fprintf(io.Out, "  Source: %s\n", colorize.Bold(source))
	fmt.Fprintf(io.Out, "  Target: %s\n", colorize.Bold(target))

	var addr = fmt.Sprintf("[%s]", migrator.PrivateIP)

	res, err := ssh.RunSSHCommand(ctx, app, dialer, addr, "migrate")
	if err != nil {
		return fmt.Errorf("error running command %w", err)
	}

	if strings.Contains(string(res), "error") {
		return fmt.Errorf("error running command %s", res)
	}

	fmt.Fprintln(io.Out, string(res))

	fmt.Fprintln(io.Out, "Import successfully completed!")

	return nil
}
