package lfsc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newImport() *cobra.Command {
	const (
		long = `Imports a local SQLite database into a LiteFS Cloud cluster.`

		short = "Import SQLite database into LiteFS Cloud"

		usage = "import"
	)

	cmd := command.New(usage, short, long, runImport,
		command.RequireSession,
		command.LoadAppNameIfPresentNoFlag,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.String{
			Name:        "input",
			Description: "Input filename",
		},
		urlFlag(),
		clusterFlag(),
		databaseFlag(),
		flag.Org(),
		flag.JSONOutput(),
	)

	return cmd
}

func runImport(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out

	clusterName := flag.GetString(ctx, "cluster")
	if clusterName == "" {
		return errors.New("required: --cluster NAME")
	}
	databaseName := flag.GetString(ctx, "database")
	if databaseName == "" {
		return errors.New("required: --database NAME")
	}

	input := flag.GetString(ctx, "input")
	if input == "" {
		return errors.New("required: --input PATH")
	}

	lfscClient, err := newLFSCClient(ctx, clusterName)
	if err != nil {
		return err
	}

	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	startTime := time.Now()

	if _, err := lfscClient.ImportDatabase(ctx, databaseName, f); err != nil {
		return err
	}

	fmt.Fprintf(out, "Database imported to %s in %s\n",
		databaseName, time.Since(startTime).Truncate(time.Millisecond))

	return nil
}
