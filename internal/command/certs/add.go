package certs

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newAdd() *cobra.Command {
	const (
		long = `The ADD command will add a certificate to an application.
		`

		short = "Add certificate"
	)

	add := command.New("add", short, long, RunAdd)

	add.Args = cobra.ExactArgs(1)

	return add
}

func RunAdd(ctx context.Context) error {
	return nil
}
