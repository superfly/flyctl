package lfsc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newRestore() *cobra.Command {
	const (
		long = `Restores a LiteFS Cloud database to a previous state.`

		short = "Restore LiteFS Cloud database"

		usage = "restore"
	)

	cmd := command.New(usage, short, long, runRestore,
		command.RequireSession,
		command.LoadAppNameIfPresentNoFlag,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.String{
			Name:        "timestamp",
			Description: "Time to restore to (ISO 8601)",
		},
		urlFlag(),
		clusterFlag(),
		databaseFlag(),
		flag.Org(),
		flag.JSONOutput(),
	)

	return cmd
}

func runRestore(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out

	clusterName := flag.GetString(ctx, "cluster")
	if clusterName == "" {
		return errors.New("required: --cluster NAME")
	}
	databaseName := flag.GetString(ctx, "database")
	if databaseName == "" {
		return errors.New("required: --database NAME")
	}

	timestampStr := flag.GetString(ctx, "timestamp")
	if timestampStr == "" {
		return errors.New("required: --timestamp YYYY-MM-DDTHH:MM:SSZ")
	}
	timestamp, err := time.Parse(time.RFC3339, timestampStr)
	if err != nil {
		return errors.New("invalid timestamp format, please use ISO 8601 format (YYYY-MM-DDTHH:MM:SSZ)")
	}

	lfscClient, err := newLFSCClient(ctx, clusterName)
	if err != nil {
		return err
	}

	startTime := time.Now()

	if _, err := lfscClient.RestoreDatabaseToTimestamp(ctx, databaseName, timestamp); err != nil {
		return err
	}

	fmt.Fprintf(out, "Database restored to previous state in %s\n",
		time.Since(startTime).Truncate(time.Millisecond))

	return nil
}
