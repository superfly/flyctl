package mpg

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a MPG database using psql`

		short = long
		usage = "connect <CLUSTER ID>"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession, command.RequireUiex)

	flag.Add(cmd,
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The database to connect to",
		},
	)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	localProxyPort := "16380"

	cluster, params, credentials, err := getMpgProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
	}

	if cluster.Status != "ready" {
		fmt.Fprintf(io.ErrOut, "%s Cluster is not in ready state, currently: %s\n", aurora.Yellow("WARN"), cluster.Status)
	}

	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		fmt.Fprintf(io.Out, "Could not find psql in your $PATH. Install it or point your psql at: %s", "someurl")
		return
	}

	err = proxy.Start(ctx, params)
	if err != nil {
		return err
	}

	user := credentials.User
	password := credentials.Password

	// Database selection priority: flag > prompt result (if interactive) > credentials.DBName
	db := credentials.DBName

	if database := flag.GetString(ctx, "database"); database != "" {
		// Priority 1: Use flag if provided
		db = database
	} else if io.IsInteractive() {
		// Priority 2: Prompt for selection if interactive
		uiexClient := uiexutil.ClientFromContext(ctx)
		databasesResponse, err := uiexClient.ListDatabases(ctx, cluster.Id)
		if err != nil {
			return fmt.Errorf("failed to list databases: %w", err)
		}

		if len(databasesResponse.Data) > 0 {
			var dbOptions []string
			for _, database := range databasesResponse.Data {
				dbOptions = append(dbOptions, database.Name)
			}

			var dbIndex int
			err = prompt.Select(ctx, &dbIndex, "Select database:", "", dbOptions...)
			if err != nil {
				return err
			}

			db = databasesResponse.Data[dbIndex].Name
		}
		// If no databases found or not interactive, db remains credentials.DBName (Priority 3)
	}

	connectUrl := fmt.Sprintf("postgresql://%s:%s@localhost:%s/%s", user, password, localProxyPort, db)
	cmd := exec.CommandContext(ctx, psqlPath, connectUrl)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	cmd.Start()
	cmd.Wait()

	return
}
