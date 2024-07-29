package webauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/azazeal/pause"
	"github.com/briandowns/spinner"
	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func SaveToken(ctx context.Context, token string) error {

	if ac, err := agent.DefaultClient(ctx); err == nil {
		_ = ac.Kill(ctx)
	}
	config.Clear(state.ConfigFile(ctx))

	if err := persistAccessToken(ctx, token); err != nil {
		return err
	}

	user, err := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		AccessToken: token,
	}).GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed retrieving current user: %w", err)
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "successfully logged in as %s\n", colorize.Bold(user.Email))

	return nil
}

func RunWebLogin(ctx context.Context, signup bool) (string, error) {
	auth, err := fly.StartCLISessionWebAuth(state.Hostname(ctx), signup)
	if err != nil {
		return "", err
	}

	io := iostreams.FromContext(ctx)
	if err := open.Run(auth.URL); err != nil {
		fmt.Fprintf(io.ErrOut,
			"failed opening browser. Copy the url (%s) into a browser and continue\n",
			auth.URL,
		)
	}

	logger := logger.FromContext(ctx)

	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "Opening %s ...\n\n", colorize.Bold(auth.URL))

	token, err := waitForCLISession(ctx, logger, io.ErrOut, auth.ID)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "", errors.New("Login expired, please try again")
	case err != nil:
		return "", err
	case token == "":
		return "", errors.New("failed to log in, please try again")
	}

	return token, nil
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
		if token, err = fly.GetAccessTokenForCLISession(ctx, id); err != nil {
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
