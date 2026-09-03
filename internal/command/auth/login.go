package auth

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command/auth/webauth"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
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
		token, err = webauth.RunWebLogin(ctx, false)
	}
	if err != nil {
		return err
	}

	if err := webauth.SaveToken(ctx, token); err != nil {
		return err
	}

	if warning := loginTokenOverrideWarning(); warning != "" {
		io := iostreams.FromContext(ctx)
		colorize := io.ColorScheme()
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "\n%s %s\n", colorize.WarningIcon(), colorize.Yellow(warning))
	}

	return nil
}

func loginTokenOverrideWarning() string {
	warnFor := func(envVar string) string {
		return fmt.Sprintf(
			"Environment variable %s is set, so flyctl will continue using it instead of the credentials just saved. Unset %s to use the new credentials.",
			envVar, envVar,
		)
	}

	// Keep this precedence in sync with config.applyEnv. An explicitly empty
	// FLY_ACCESS_TOKEN prevents FLY_API_TOKEN from being selected.
	if token, ok := os.LookupEnv(config.AccessTokenEnvKey); ok {
		if token == "" {
			return ""
		}

		return warnFor(config.AccessTokenEnvKey)
	}

	if token := os.Getenv(config.APITokenEnvKey); token != "" {
		return warnFor(config.APITokenEnvKey)
	}

	return ""
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

	token, err := fly.GetAccessToken(ctx, email, password, otp)
	if err != nil {
		err = fmt.Errorf("failed retrieving access token: %w", err)

		return "", err
	}

	return token, nil
}
