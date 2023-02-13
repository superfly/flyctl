package certs

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newShow() *cobra.Command {
	const (
		long = `The SHOW command will show a certificate.
		`

		short = "Show certificate"
	)

	show := command.New("show", short, long, RunShow)

	show.Args = cobra.ExactArgs(1)

	return show
}

func RunShow(ctx context.Context) error {
	return nil
}
