package apps

import (
	"context"
	"fmt"
	"time"

	"github.com/azazeal/pause"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/client"
)

func newResume() *cobra.Command {
	const (
		long = `The APPS RESUME command will restart a previously suspended application. 
The application will resume with its original region pool and a min count of one
meaning there will be one running instance once restarted. Use SCALE SET MIN= to raise
the number of configured instances.
`

		short = "Resume an application"

		usage = "resume [APPNAME]"
	)

	resume := command.New(usage, short, long, runResume,
		command.RequireSession)

	resume.Args = cobra.RangeArgs(1, 1)

	return resume
}

func runResume(ctx context.Context) (err error) {
	appName := flag.FirstArg(ctx)

	client := client.FromContext(ctx).API()

	var app *api.App
	if app, err = client.ResumeApp(ctx, appName); err != nil {
		err = fmt.Errorf("failed resuming %s: %w", appName, err)

		return
	}

	io := iostreams.FromContext(ctx)
	if !io.IsInteractive() {
		waitUntilAppIsRunning(ctx, app, &err)

		return
	}

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = io.ErrOut
	s.Prefix = fmt.Sprintf("resuming %s with 1 instance to start ", app.Name)
	s.Start()

	if waitUntilAppIsRunning(ctx, app, &err); err != nil {
		s.Stop()

		err = fmt.Errorf("failed retrieving %s status: %w", appName, err)

		return
	}

	s.FinalMSG = fmt.Sprintf("resume complete - %s is now %s with 1 running instance\n",
		app.Name, app.Status)
	s.Stop()

	return err
}

func waitUntilAppIsRunning(ctx context.Context, app *api.App, err *error) {
	client := client.FromContext(ctx).API()

	for err == nil && app.Status != "running" {
		app, *err = client.GetApp(ctx, app.Name)

		pause.For(ctx, time.Millisecond*100)
	}
}
