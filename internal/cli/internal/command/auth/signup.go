package auth

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/client"
)

func newSignup() *cobra.Command {
	const (
		long = `Creates a new fly account. The command opens the browser 
and sends the user to a form to provide appropriate credentials.
`
		short = "Create a new fly account"
	)

	return command.New("signup", short, long, runSignup)
}

func runSignup(ctx context.Context) error {
	name := state.Hostname(ctx)

	cliAuth, err := api.StartCLISessionWebAuth(name, true)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	if err := open.Run(cliAuth.AuthURL); err != nil {
		fmt.Fprintf(io.ErrOut, "Error opening browser. Copy the url %s into a browser and continue.\n", cliAuth.AuthURL)
	}

	select {
	case <-time.After(15 * time.Minute):
		return errors.New("login expired, please try again")
	case cliAuth = <-waitForCLISession(cliAuth.ID):
	}

	if cliAuth.AccessToken == "" {
		return errors.New("login failed. please try again")
	}

	cfg := config.FromContext(ctx)
	cfg.SetAccessToken(cliAuth.AccessToken)

	c := client.FromContext(ctx)
	if !c.InitApi() {
		return client.ErrNoAuthToken
	}

	user, err := c.API().GetCurrentUser(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "successfully logged in as", aurora.Bold(user.Email))

	return nil
}

func waitForCLISession(id string) <-chan api.CLISessionAuth {
	done := make(chan api.CLISessionAuth)

	go func() {
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Writer = os.Stderr
		s.Prefix = "Waiting for session..."
		s.FinalMSG = "Waiting for session...Done\n"
		s.Start()
		defer s.Stop()

		for {
			time.Sleep(1 * time.Second)
			cliAuth, _ := api.GetAccessTokenForCLISession(id)

			if cliAuth.AccessToken != "" {
				done <- cliAuth
				break
			}
		}
	}()

	return done
}
