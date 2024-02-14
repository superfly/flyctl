package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
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

		err   error
		token string
	)

	switch {
	case interactive, email != "", password != "", otp != "":
		token, err = runShellLogin(ctx, email, password, otp)
	default:
		token, err = runWebLogin(ctx, false)
	}
	if err != nil {
		return err
	}

	if ac, err := agent.DefaultClient(ctx); err == nil {
		_ = ac.Kill(ctx)
	}
	config.Clear(state.ConfigFile(ctx))

	if err := persistAccessToken(ctx, token); err != nil {
		return err
	}

	user, err := client.FromToken(token).API().GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "successfully logged in as %s\n", colorize.Bold(user.Email))

	return nil
}

type requiredWhenNonInteractive string

func (r requiredWhenNonInteractive) Error() string {
	return fmt.Sprintf("%s must be specified when not running interactively", string(r))
}

func runShellLogin(ctx context.Context, email, password, otp string) (string, error) {
	if email == "" {
		switch err := prompt.String(ctx, &email, "Email:", "", true); {
		case err == nil:
			break
		case prompt.IsNonInteractive(err):
			return "", requiredWhenNonInteractive("email")
		default:
			return "", err
		}
	}

	if password == "" {
		switch err := prompt.Password(ctx, &password, "Password:", true); {
		case err == nil:
			break
		case prompt.IsNonInteractive(err):
			return "", requiredWhenNonInteractive("password")
		default:
			return "", err
		}
	}

	if otp == "" {
		switch err := prompt.String(ctx, &otp, "One Time Password (if any):", "", false); {
		case err == nil:
			break
		case prompt.IsNonInteractive(err):
			break
		default:
			return "", err
		}
	}

	token, err := api.GetAccessToken(ctx, email, password, otp)
	if err != nil {
		err = fmt.Errorf("failed retrieving access token: %w", err)

		return "", err
	}

	return token, nil
}
