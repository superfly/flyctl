package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newRollingRestart() (cmd *cobra.Command) {
	const (
		long = `Performs a rolling restart against the target Postgres cluster
`
		short = "Perform a rolling restart"
		usage = "rolling-restart"
	)

	cmd = command.New(usage, short, long, runRollingRestart,
		command.RequireSession,
		command.RequireAppName,
	)
	// cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func runRollingRestart(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	client := client.FromContext(ctx).API()

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	machines, err := client.ListMachines(ctx, app.ID, "started")
	if err != nil {
		return err
	}

	pgCmd, err := newPostgresCmd(ctx, app)
	if err != nil {
		return err
	}

	roleMap := map[string][]*api.Machine{}
	io := iostreams.FromContext(ctx)

	// Collect PG role information from each machine
	for _, machine := range machines {
		fmt.Fprintf(io.Out, "Identifying role of Machine %q... ", machine.ID)

		role, err := pgCmd.getRole(machine)
		if err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "%s\n", role)
		roleMap[role] = append(roleMap[role], machine)
	}

	for _, machine := range roleMap["replica"] {
		fmt.Fprintf(io.Out, "Restarting machine %q... ", machine.ID)
		if err = pgCmd.restartNode(machine); err != nil {
			fmt.Fprintln(io.Out, "failed")
			return err
		}
		fmt.Fprintln(io.Out, "complete")
	}

	for _, machine := range roleMap["leader"] {
		fmt.Printf("Stepping down leader %q... ", machine.ID)
		if err = pgCmd.failover(); err != nil {
			fmt.Fprintln(io.Out, "failed")
			fmt.Fprintln(io.Out, err.Error())
		} else {
			fmt.Fprintln(io.Out, "complete")
		}

		fmt.Fprintf(io.Out, "Restarting machine %q... ", machine.ID)
		if err = pgCmd.restartNode(machine); err != nil {
			fmt.Fprintln(io.Out, "failed")
			return err
		}
		fmt.Fprintln(io.Out, "complete")
	}

	return nil
}
