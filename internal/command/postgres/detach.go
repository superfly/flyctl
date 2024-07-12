package postgres

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newDetach() *cobra.Command {
	const (
		short = "Detach a Fly Postgres cluster from a Fly App."
		long  = short + " Learn more: https://fly.io/docs/postgres/managing/attach-detach/"
		usage = "detach <POSTGRES APP>"
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
		client = flyutil.ClientFromContext(ctx)

		pgAppName = flag.FirstArg(ctx)
		appName   = appconfig.NameFromContext(ctx)
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
	return runMachineDetach(ctx, app, pgApp)
}

func runMachineDetach(ctx context.Context, app *fly.AppCompact, pgApp *fly.AppCompact) error {
	var (
		MinPostgresHaVersion         = "0.0.19"
		MinPostgresFlexVersion       = "0.0.3"
		MinPostgresStandaloneVersion = "0.0.7"
	)

	machines, err := mach.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("machines could not be retrieved %w", err)
	}

	if err := hasRequiredVersionOnMachines(machines, MinPostgresHaVersion, MinPostgresFlexVersion, MinPostgresStandaloneVersion); err != nil {
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

// TODO - This process needs to be re-written to suppport non-interactive terminals.
func detachAppFromPostgres(ctx context.Context, leaderIP string, app *fly.AppCompact, pgApp *fly.AppCompact) error {
	var (
		client = flyutil.ClientFromContext(ctx)
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

	input := fly.DetachPostgresClusterInput{
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
