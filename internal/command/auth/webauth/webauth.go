package webauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/azazeal/pause"
	"github.com/briandowns/spinner"
	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

const tokensHelpURL = "https://fly.io/docs/security/tokens/"

// loginTimeout is how long a login waits for the browser approval. The
// server keeps a CLI session for the same 15 minutes.
const loginTimeout = 15 * time.Minute

// errCI is returned on CI, where nobody will ever approve a browser login:
// fail at once rather than wait out the timeout.
func errCI(command string) error {
	return fmt.Errorf("fly auth %s cannot run on CI. Set FLY_API_TOKEN to a token instead: %s", command, tokensHelpURL)
}

// errHeadlessNoListener describes a run where neither delivery path for the
// completion code is available: no terminal to paste it into and no
// loopback listener for the browser to post it to.
func errHeadlessNoListener(command string) error {
	return fmt.Errorf("fly auth %s needs either an interactive terminal or a free loopback port for the browser to call back on, and has neither. Set FLY_API_TOKEN to a token instead: %s", command, tokensHelpURL)
}

// errHeadlessLegacyServer describes a server that predates PKCE: the only
// login flow it offers is unsafe to run without someone watching.
func errHeadlessLegacyServer(command string) error {
	return fmt.Errorf("fly auth %s needs an interactive terminal against this server. Set FLY_API_TOKEN to a token instead: %s", command, tokensHelpURL)
}

// headlessNotice explains, to whoever is reading a non-interactive run, why
// the command might appear to hang: the browser has to be on this machine,
// and an agent has to keep the command alive for longer than its usual
// command timeout.
func headlessNotice(command, url string) string {
	return fmt.Sprintf("Open this URL in a browser and approve the %s:\n\n    %s\n\n", command, url) +
		"This terminal is not interactive, so the code cannot be pasted here.\n" +
		fmt.Sprintf("The %s completes on its own once approved in a browser on this machine, within %d minutes.\n", command, int(loginTimeout.Minutes())) +
		"Keep this command running until then. Agents: run it in the background, since command timeouts are usually shorter.\n" +
		"If the browser is on another machine, or this command was stopped, finish with: fly auth login --code <code shown after approving>\n" +
		"This command exits on its own once that succeeds.\n\n"
}

func SaveToken(ctx context.Context, token string) error {

	if ac, err := agent.DefaultClient(ctx); err == nil {
		_ = ac.Kill(ctx)
	}
	config.Clear(state.ConfigFile(ctx))

	if err := persistAccessToken(ctx, token); err != nil {
		return err
	}

	// Record the login timestamp
	if err := config.SetLastLogin(state.ConfigFile(ctx), time.Now()); err != nil {
		return fmt.Errorf("failed persisting login timestamp: %w", err)
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
	args := map[string]any{
		"signup": signup,
		"target": "auth",
	}

	var (
		lockOrg      = os.Getenv("FLY_TOKEN_LOCK_ORG")
		lockApp      = os.Getenv("FLY_TOKEN_LOCK_APP")
		lockInstance = os.Getenv("FLY_TOKEN_LOCK_INSTANCE")
		metadata     map[string]any
	)

	if lockOrg != "" || lockApp != "" || lockInstance != "" {
		metadata = map[string]any{}
		args["metadata"] = metadata
	}
	if lockOrg != "" {
		metadata["lock_organization"] = lockOrg
	}
	if lockApp != "" {
		metadata["lock_app"] = lockApp
	}
	if lockInstance != "" {
		metadata["lock_instance"] = lockInstance
	}

	io := iostreams.FromContext(ctx)
	logger := logger.FromContext(ctx)

	command := "login"
	if signup {
		command = "signup"
	}

	headless := !io.IsStdinTTY()
	if headless && env.IsCI() {
		return "", errCI(command)
	}

	pkce, err := newPKCELogin(args)
	if err != nil {
		return "", err
	}

	// No terminal means no pasting, so the loopback callback must be available.
	if headless && pkce.port == 0 {
		pkce.close()

		return "", errHeadlessNoListener(command)
	}

	auth, err := fly.StartCLISession(state.Hostname(ctx), args)
	if err != nil {
		pkce.close()

		return "", err
	}

	// The pre-PKCE flow hands the token to whoever polls with the session
	// id, so never run it unattended.
	if headless && !auth.PKCE {
		pkce.close()

		return "", errHeadlessLegacyServer(command)
	}

	// Save the login so `fly auth login --code` can finish it if this
	// process is stopped or the browser can't reach the callback.
	pendingPath := pendingLoginPath(ctx)
	expiresAt := time.Now().Add(loginTimeout)
	var watch *pendingWatch
	if auth.PKCE {
		pending := pendingLogin{ID: auth.ID, Verifier: pkce.verifier, ExpiresAt: expiresAt}
		if err := savePendingLogin(pendingPath, pending); err != nil {
			fmt.Fprintf(io.ErrOut, "Could not save this login (%v), so `fly auth login --code` won't be able to finish it.\n", err)
		} else {
			configFile := state.ConfigFile(ctx)
			token, _ := config.ReadAccessToken(configFile)
			watch = &pendingWatch{path: pendingPath, configFile: configFile, id: auth.ID, token: token}
		}
	}

	colorize := io.ColorScheme()
	openErr := open.Run(auth.URL)
	switch {
	case headless:
		// Whether or not a browser opened, the reader may have to pass the
		// URL on to someone, so the notice gives it a line of its own.
		fmt.Fprint(io.ErrOut, headlessNotice(command, auth.URL))
	case openErr != nil:
		fmt.Fprintf(io.ErrOut,
			"failed opening browser. Copy the url (%s) into a browser and continue\n\n",
			colorize.Bold(auth.URL),
		)
	default:
		fmt.Fprintf(io.Out, "Opening %s ...\n\n", colorize.Bold(auth.URL))
	}

	var token string
	if auth.PKCE {
		token, err = waitForPKCEToken(ctx, io, logger, auth.ID, pkce, !headless, watch)
		// Finished, or past its 15 minutes: nothing is left for --code to do.
		// A login stopped earlier, by Ctrl-C or a caller's own deadline, keeps
		// its file, which is what --code resumes.
		if err == nil || time.Now().After(expiresAt) {
			removePendingLogin(pendingPath, auth.ID)
		}
	} else {
		// Server predates the PKCE flow
		pkce.close()
		token, err = waitForCLISession(ctx, logger, io.ErrOut, auth.ID)
	}

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
	ctx, cancel := context.WithTimeoutCause(parent, loginTimeout, fmt.Errorf("waiting for CLI login: %w", context.DeadlineExceeded))
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
