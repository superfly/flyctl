package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newDetach() *cobra.Command {
	const (
		short = "Detach a postgres cluster from an app"
		long  = short + "\n"
		usage = "detach [POSTGRES APP]"
	)

	cmd := command.New(usage, short, long, runDetach,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runDetach(ctx context.Context) error {
	var (
		client = client.FromContext(ctx).API()

		pgAppName = flag.FirstArg(ctx)
		appName   = app.NameFromContext(ctx)
	)

	pgApp, err := client.GetAppCompact(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("get postgres app: %w", err)
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	ctx, err = apps.BuildContext(ctx, pgApp)
	if err != nil {
		return err
	}

	switch pgApp.PlatformVersion {
	case "machines":
		return runMachineDetach(ctx, app, pgApp)
	case "nomad":
		return runNomadDetach(ctx, app, pgApp)
	default:
		return fmt.Errorf("unknown platform version")
	}
}

func runMachineDetach(ctx context.Context, app *api.AppCompact, pgApp *api.AppCompact) error {
	var (
		MinPostgresHaVersion = "0.0.19"
	)

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	if len(machines) == 0 {
		return fmt.Errorf("no 6pn ips founds for %s app", pgApp.Name)
	}

	leader, err := pickLeader(ctx, machines)
	if err != nil {
		return err
	}

	return detachAppFromPostgres(ctx, leader.PrivateIP, app, pgApp)

}

func runNomadDetach(ctx context.Context, app *api.AppCompact, pgApp *api.AppCompact) error {
	var (
		MinPostgresHaVersion = "0.0.19"
		client               = client.FromContext(ctx).API()
	)

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return fmt.Errorf("can't establish agent %w", err)
	}

	if err := hasRequiredVersionOnNomad(pgApp, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
		return err
	}

	pgInstances, err := agentclient.Instances(ctx, pgApp.Organization.Slug, pgApp.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup 6pn ip for %s app: %v", pgApp.Name, err)
	}

	if len(pgInstances.Addresses) == 0 {
		return fmt.Errorf("no 6pn ips found for %s app", pgApp.Name)
	}

	leaderIP, err := leaderIpFromNomadInstances(ctx, pgInstances.Addresses)
	if err != nil {
		return err
	}

	return detachAppFromPostgres(ctx, leaderIP, app, pgApp)
}

// TODO - This process needs to be re-written to suppport non-interactive terminals.
func detachAppFromPostgres(ctx context.Context, leaderIP string, app *api.AppCompact, pgApp *api.AppCompact) error {
	var (
		client = client.FromContext(ctx).API()
		dialer = agent.DialerFromContext(ctx)
		io     = iostreams.FromContext(ctx)
	)

	attachments, err := client.ListPostgresClusterAttachments(ctx, app.ID, pgApp.ID)
	if err != nil {
		return err
	}

	if len(attachments) == 0 {
		return fmt.Errorf("no attachments found")
	}

	selected := 0
	msg := "Select the attachment that you would like to detach (Database will remain intact): "
	options := []string{}
	for _, opt := range attachments {
		str := fmt.Sprintf("PG Database: %s, PG User: %s, Environment variable: %s",
			opt.DatabaseName,
			opt.DatabaseUser,
			opt.EnvironmentVariableName,
		)
		options = append(options, str)
	}
	if err = prompt.Select(ctx, &selected, msg, "", options...); err != nil {
		return err
	}

	targetAttachment := attachments[selected]

	pgclient := flypg.NewFromInstance(leaderIP, dialer)

	// Remove user if exists
	exists, err := pgclient.UserExists(ctx, targetAttachment.DatabaseUser)
	if err != nil {
		return err
	}
	if exists {
		err := pgclient.DeleteUser(ctx, targetAttachment.DatabaseUser)
		if err != nil {
			return fmt.Errorf("error running user-delete: %w", err)
		}
	}

	// Remove secret from consumer app.
	_, err = client.UnsetSecrets(ctx, app.Name, []string{targetAttachment.EnvironmentVariableName})
	if err != nil {
		// This will error if secret doesn't exist, so just send to stdout.
		fmt.Fprintln(io.Out, err.Error())
	} else {
		fmt.Fprintf(io.Out, "Secret %q was scheduled to be removed from app %s\n",
			targetAttachment.EnvironmentVariableName,
			app.Name,
		)
	}

	input := api.DetachPostgresClusterInput{
		AppID:                       app.Name,
		PostgresClusterId:           pgApp.Name,
		PostgresClusterAttachmentId: targetAttachment.ID,
	}

	if err = client.DetachPostgresCluster(ctx, input); err != nil {
		return err
	}
	fmt.Fprintln(io.Out, "Detach completed successfully!")

	return nil
}
