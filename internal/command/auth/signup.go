package auth

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command/auth/webauth"

	"github.com/superfly/flyctl/internal/command"
)

func newSignup() *cobra.Command {
	const (
		long = `Create a new Fly.io account. The command opens a sign-up page in your
browser (or prints its URL if it can't) and waits for you to finish. When
you do, you're logged in and the access token is saved to your fly config.

It can't run in CI without a terminal; set ` + "`FLY_API_TOKEN`" + ` there
instead. If you already have an account, use ` + "`fly auth login`" + `.`
		short = "Create a new Fly.io account"
	)

	return command.New("signup", short, long, runSignup)
}

func runSignup(ctx context.Context) error {
	token, err := webauth.RunWebLogin(ctx, true)
	if err != nil {
		return err
	}

	if err := webauth.SaveToken(ctx, token); err != nil {
		return err
	}

	return nil
}
