package postgres

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/flypg"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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
		MinPostgresHaVersion = "0.0.19"
		appName              = app.NameFromContext(ctx)
		pgAppName            = flag.FirstArg(ctx)
		client               = client.FromContext(ctx).API()
	)

	app, err := client.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	pgApp, err := client.GetAppCompact(ctx, pgAppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	switch pgApp.PlatformVersion {
	case "nomad":
		if err := hasRequiredVersionOnNomad(pgApp, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
	case "machines":
		agentclient, err := agent.Establish(ctx, client)
		if err != nil {
			return fmt.Errorf("can't establish agent %w", err)
		}

		dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
		if err != nil {
			return fmt.Errorf("can't build tunnel for %s: %s", app.Organization.Slug, err)
		}
		ctx = agent.DialerWithContext(ctx, dialer)

		flapsClient, err := flaps.New(ctx, pgApp)
		if err != nil {
			return fmt.Errorf("list of machines could not be retrieved: %w", err)
		}

		members, err := flapsClient.List(ctx, "started")
		if err != nil {
			return fmt.Errorf("machines could not be retrieved %w", err)
		}
		leader, err := fetchPGLeader(ctx, pgApp, members)
		if err != nil {
			return fmt.Errorf("can't fetch leader: %w", err)
		}
		if err := hasRequiredVersionOnMachines(leader, MinPostgresHaVersion, MinPostgresHaVersion); err != nil {
			return err
		}
	}

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

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, pgApp.Organization.Slug)
	if err != nil {
		return fmt.Errorf("can't build tunnel for %s: %w", app.Organization.Slug, err)
	}

	pgclient := flypg.New(pgAppName, dialer)

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

	io := iostreams.FromContext(ctx)

	// Remove secret from consumer app.
	_, err = client.UnsetSecrets(ctx, appName, []string{targetAttachment.EnvironmentVariableName})
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
		AppID:                       appName,
		PostgresClusterId:           pgAppName,
		PostgresClusterAttachmentId: targetAttachment.ID,
	}

	if err = client.DetachPostgresCluster(ctx, input); err != nil {
		return err
	}
	fmt.Fprintln(io.Out, "Detach completed successfully!")

	return nil
}
