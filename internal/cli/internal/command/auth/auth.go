// Package auth implements the auth command chain.
package auth

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/azazeal/pause"
	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/logger"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	const (
		long = `Authenticate with Fly (and logout if you need to).
If you do not have an account, start with the AUTH SIGNUP command.
If you do have and account, begin with the AUTH LOGIN subcommand.
`
		short = "Manage authentication"
	)

	auth := command.New("auth", short, long, nil)

	auth.AddCommand(
		newWhoAmI(),
		newToken(),
		newLogin(),
		newDocker(),
		newLogout(),
		newSignup(),
	)

	return auth
}

func runWebLogin(ctx context.Context, signup bool) error {
	cliAuth, err := api.StartCLISessionWebAuth(state.Hostname(ctx), signup)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	if err := open.Run(cliAuth.AuthURL); err != nil {
		fmt.Fprintf(io.ErrOut,
			"failed opening browser. Copy the url (%s) into a browser and continue\n",
			cliAuth.AuthURL,
		)
	}

	logger := logger.FromContext(ctx)

	token, err := waitForCLISession(ctx, logger, io.ErrOut, cliAuth.ID)
	switch {
	case err == nil:
		break
	case errors.Is(err, context.DeadlineExceeded):
		return errors.New("Login expired, please try again")
	case token == "":
		return errors.New("failed to log in, please try again")
	default:
		return err
	}

	if err := persistAccessToken(ctx, token); err != nil {
		return err
	}

	client := client.FromToken(token).API()

	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	fmt.Fprintf(io.Out, "successfully logged in as %s\n", aurora.Bold(user.Email))

	return nil
}

// TODO: this does NOT break on interrupts
func waitForCLISession(parent context.Context, logger *logger.Logger, w io.Writer, id string) (token string, err error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Minute)
	defer cancel()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = w
	s.Prefix = "Waiting for session..."
	s.Start()

	for ctx.Err() == nil {
		if token, err = api.GetAccessTokenForCLISession(ctx, id); err != nil {
			logger.Debugf("failed retrieving token: %v", err)

			pause.For(ctx, time.Second)

			continue
		}

		logger.Debug("retrieved access token.")

		s.FinalMSG = "Waiting for session... Done\n"
		s.Stop()

		break
	}

	return
}

func persistAccessToken(ctx context.Context, token string) (err error) {
	path := state.ConfigFile(ctx)

	if err = config.SetAccessToken(path, token); err != nil {
		err = fmt.Errorf("failed persisting %s in %s: %w\n",
			config.AccessTokenFileKey, path, err)
	}

	return
}
