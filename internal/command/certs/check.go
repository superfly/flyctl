package certs

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newCheck() *cobra.Command {
	const (
		long = `The CHECK command will check the validity of a certificate.
		`
		short = "Check certificate"
	)

	check := command.New("check", short, long, RunCheck)

	check.Args = cobra.ExactArgs(1)

	return check
}

func RunCheck(ctx context.Context) error {
	return nil
}
