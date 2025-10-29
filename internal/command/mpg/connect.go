package mpg

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newConnect() (cmd *cobra.Command) {
	const (
		long = `Connect to a MPG database using psql`

		short = long
		usage = "connect"
	)

	cmd = command.New(usage, short, long, runConnect, command.RequireSession)

	flag.Add(cmd,
		flag.MPGCluster(),
		flag.String{
			Name:        "database",
			Shorthand:   "d",
			Description: "The database to connect to",
		},
	)

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
	db := credentials.DBName

	// Override database name if provided via flag
	if database := flag.GetString(ctx, "database"); database != "" {
		db = database
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
