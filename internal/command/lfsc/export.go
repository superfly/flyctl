package lfsc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newExport() *cobra.Command {
	const (
		long = `Exports the current state of the database to a file.`

		short = "Export LiteFS Cloud database"

		usage = "export"
	)

	cmd := command.New(usage, short, long, runExport,
		command.RequireSession,
		command.LoadAppNameIfPresentNoFlag,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.String{
			Name:        "output",
			Description: "Output filename",
		},
		flag.Bool{
			Name:        "force",
			Shorthand:   "f",
			Description: "Overwrite output file if it already exists",
		},
		urlFlag(),
		clusterFlag(),
		databaseFlag(),
		flag.Org(),
		flag.JSONOutput(),
	)

	return cmd
}

func runExport(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out
	output := flag.GetString(ctx, "output")

	clusterName := flag.GetString(ctx, "cluster")
	if clusterName == "" {
		return errors.New("required: --cluster NAME")
	}
	databaseName := flag.GetString(ctx, "database")
	if databaseName == "" {
		return errors.New("required: --database NAME")
	}

	if output == "" {
		return errors.New("required: --output PATH")
	}
	force := flag.GetBool(ctx, "force")

	if !force {
		if _, err := os.Stat(output); err == nil {
			return errors.New("output file already exists, use --force to overwrite")
		}
	}

	lfscClient, err := newLFSCClient(ctx, clusterName)
	if err != nil {
		return err
	}

	startTime := time.Now()

	rc, err := lfscClient.ExportDatabase(ctx, databaseName)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	f, err := os.Create(output)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, rc); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	fmt.Fprintf(out, "Database exported to %s in %s\n",
		output, time.Since(startTime).Truncate(time.Millisecond))

	return nil
}
