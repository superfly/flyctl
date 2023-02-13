package certs

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newRemove() *cobra.Command {
	const (
		long = `The REMOVE command will remove a certificate from an application.
		`

		short = "Remove certificate"
	)

	remove := command.New("remove", short, long, RunRemove)

	remove.Args = cobra.ExactArgs(1)

	return remove
}

func RunRemove(ctx context.Context) error {
	return nil
}
