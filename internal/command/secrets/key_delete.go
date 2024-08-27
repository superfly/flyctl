package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newKeyDelete() (cmd *cobra.Command) {
	const (
		long = `Delete the application key secret by label. The label must be fully qualified
with the key version.`
		short = `Delete the application key secret`
		usage = "delete [flags] label"
	)

	cmd = command.New(usage, short, long, runKeyDelete, command.RequireSession, command.RequireAppName)

	cmd.Aliases = []string{"rm"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runKeyDelete(ctx context.Context) (err error) {
	label := flag.Args(ctx)[0]
	flapsClient, err := getFlapsClient(ctx)
	if err != nil {
		return err
	}

	err = flapsClient.DeleteSecret(ctx, label)
	if err != nil {
		var ferr *flaps.FlapsError
		if errors.As(err, &ferr) && ferr.ResponseStatusCode == 404 {
			return fmt.Errorf("secret label %v is not found", label)
		}
		return err
	}
	return nil
}
