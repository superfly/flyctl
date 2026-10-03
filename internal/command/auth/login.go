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
		flag.Bool{
			Name:        "restricted",
			Description: "Save org tokens with limited permissions instead of a full user token",
		},
		flag.StringSlice{
			Name:        "org",
			Description: "With --restricted, organization slugs to grant access to",
		},
		flag.String{
			Name:        "mask",
			Description: "With --restricted, permissions to allow: any of r, w, c, d, C",
		},
		flag.Duration{
			Name:        "expiry",
			Description: "With --restricted, how long the tokens are valid",
			Default:     restrictedDefaultExpiry,
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

	if err := restrictedFlagsWithoutRestricted(ctx); err != nil {
		return err
	}

	switch {
	case interactive, email != "", password != "", otp != "":
		token, err = runShellLogin(ctx, email, password, otp)
	default:
		token, err = webauth.RunWebLogin(ctx, false)
	}
	if err != nil {
		return err
	}

	if flag.GetBool(ctx, "restricted") {
		return saveRestrictedToken(ctx, token)
	}

	if err := webauth.SaveToken(ctx, token); err != nil {
		return err
	}

	warnLoginTokenOverride(ctx)

	return nil
}

func saveRestrictedToken(ctx context.Context, userToken string) error {
	token, err := restrictToken(ctx, userToken)
	if err != nil {
		return err
	}

	if err := webauth.SaveTokenFor(ctx, token, userToken); err != nil {
		return err
	}

	if err := revokeUserToken(ctx, userToken); err != nil {
		warn(iostreams.FromContext(ctx), fmt.Sprintf(
			"Failed revoking the unrestricted login session (%v). "+
				"It is not saved locally, but remains valid until it expires.",
			err,
		))
	}

	warnLoginTokenOverride(ctx)

	return nil
}

func warnLoginTokenOverride(ctx context.Context) {
	if warning := loginTokenOverrideWarning(); warning != "" {
		io := iostreams.FromContext(ctx)
		colorize := io.ColorScheme()
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "\n%s %s\n", colorize.WarningIcon(), colorize.Yellow(warning))
	}
}

func loginTokenOverrideWarning() string {
	warnFor := func(envVar string) string {
		return fmt.Sprintf(
			"Environment variable %s is set, so flyctl will continue using it instead of the access token just saved. Unset %s to use the new access token.",
			envVar, envVar,
		)
	}

	// Keep this precedence in sync with config.applyEnv. An explicitly empty
	// FLY_ACCESS_TOKEN prevents FLY_API_TOKEN from being selected.
	if token, ok := os.LookupEnv(config.AccessTokenEnvKey); ok {
		if token == "" {
			return ""
		}
		if apiToken := os.Getenv(config.APITokenEnvKey); apiToken != "" {
			return fmt.Sprintf(
				"Environment variables %s and %s are set. flyctl will continue using these instead of the access token just saved. Unset both variables to use the new access token.",
				config.AccessTokenEnvKey, config.APITokenEnvKey,
			)
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
