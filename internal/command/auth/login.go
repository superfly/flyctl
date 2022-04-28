package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newLogin() *cobra.Command {
	const (
		long = `Logs a user into the Fly platform. Supports browser-based,
email/password and one-time-password authentication. Defaults to using
browser-based authentication.
`
		short = "Log in a user"
	)

	cmd := command.New("login", short, long, runLogin)

	flag.Add(cmd,
		flag.Bool{
			Name:        "interactive",
			Shorthand:   "i",
			Description: "Log in with an email and password interactively",
		},
		flag.String{
			Name:        "email",
			Description: "Login email",
		},
		flag.String{
			Name:        "password",
			Description: "Login password",
		},
		flag.String{
			Name:        "otp",
			Description: "One time password",
		},
	)

	return cmd
}

func runLogin(ctx context.Context) error {
	var (
		interactive = flag.GetBool(ctx, "interactive")
		email       = flag.GetString(ctx, "email")
		password    = flag.GetString(ctx, "password")
		otp         = flag.GetString(ctx, "otp")
	)

	switch {
	case interactive, email != "", password != "", otp != "":
		return runShellLogin(ctx, email, password, otp)
	default:
		return runWebLogin(ctx, false)
	}
}

type requiredWhenNonInteractive string

func (r requiredWhenNonInteractive) Error() string {
	return fmt.Sprintf("%s must be specified when not running interactively", string(r))
}

func runShellLogin(ctx context.Context, email, password, otp string) (err error) {
	if email == "" {
		switch err = prompt.String(ctx, &email, "Email:", "", true); {
		case err == nil:
			break
		case prompt.IsNonInteractive(err):
			return requiredWhenNonInteractive("email")
		default:
			return
		}
	}

	if password == "" {
		switch err = prompt.Password(ctx, &password, "Password:", true); {
		case err == nil:
			break
		case prompt.IsNonInteractive(err):
			return requiredWhenNonInteractive("password")
		default:
			return
		}
	}

	if otp == "" {
		switch err = prompt.String(ctx, &otp, "One Time Password (if any):", "", false); {
		case err == nil:
			break
		case prompt.IsNonInteractive(err):
			err = nil
		default:
			return
		}
	}

	var token string
	if token, err = api.GetAccessToken(ctx, email, password, otp); err != nil {
		err = fmt.Errorf("failed retrieving access token: %w", err)

		return
	}

	err = persistAccessToken(ctx, token)

	return
}
