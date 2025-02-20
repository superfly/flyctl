package mpg

import (
	"context"
	"fmt"
	"os/exec"

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

	cmd = command.New(usage, short, long, runConnect, command.RequireSession, command.RequireUiex)

	flag.Add(cmd,
		flag.Org(),
	)

	return cmd
}

func runConnect(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	localProxyPort := "16380"

	cluster, params, password, err := getMpgProxyParams(ctx, localProxyPort)
	if err != nil {
		return err
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

	name := fmt.Sprintf("pgdb-%s", cluster.Id)
	connectUrl := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=require", name, password, localProxyPort, name)
	cmd := exec.CommandContext(ctx, psqlPath, connectUrl)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

	cmd.Start()
	cmd.Wait()

	return
}
