package ssh

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newSftp() *cobra.Command {
	const (
		long  = `Get or put files from a remote VM.`
		short = long
		usage = "sftp"
	)

	cmd := command.New("sftp", short, long, nil)

	cmd.AddCommand(
		newLs(),
	)

	return cmd
}

func newLs() *cobra.Command {
	const (
		long  = `tktktktk.`
		short = long
		usage = "ls"
	)

	cmd := command.New(usage, short, long, runLs, command.RequireSession, command.LoadAppNameIfPresent)

	stdArgsSSH(cmd)

	return cmd
}

func runLs(ctx context.Context) error {
	client := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	agentclient, dialer, err := bringUp(ctx, client, app)
	if err != nil {
		return err
	}

	addr, err := lookupAddress(ctx, agentclient, dialer, app, false)
	if err != nil {
		return err
	}

	params := &SSHParams{
		Ctx:            ctx,
		Org:            app.Organization,
		Dialer:         dialer,
		App:            appName,
		Stdin:          os.Stdin,
		Stdout:         os.Stdout,
		Stderr:         os.Stderr,
		DisableSpinner: true,
	}

	conn, err := sshConnect(params, addr)
	if err != nil {
		captureError(err, app)
		return err
	}

	ftp, err := sftp.NewClient(conn.Client,
		sftp.UseConcurrentReads(true),
		sftp.UseConcurrentWrites(true),
	)
	if err != nil {
		return err
	}

	root := "/"
	args := flag.Args(ctx)
	if len(args) != 0 {
		root = args[0]
	}

	walker := ftp.Walk(root)
	for walker.Step() {
		if err = walker.Err(); err != nil {
			return err
		}

		fmt.Printf(walker.Path() + "\n")
	}

	return nil
}
