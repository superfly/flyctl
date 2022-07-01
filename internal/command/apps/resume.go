package apps

import (
	"context"
	"fmt"
	"time"

	"github.com/azazeal/pause"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
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

	resume := command.New(usage, short, long, RunResume,
		command.RequireSession)

	resume.Hidden = true
	resume.Args = cobra.ExactArgs(1)

	return resume
}

// TODO: make internal once the resume package is removed
func RunResume(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	fmt.Fprintf(io.ErrOut, "Warning: this command is deprecated. Only use it if you have a previously suspended app. Use 'fly scale count 0' if you need to stop an app temporarily.\n")
	appName := flag.FirstArg(ctx)

	client := client.FromContext(ctx).API()

	var app *api.AppCompact
	if app, err = client.ResumeApp(ctx, appName); err != nil {
		err = fmt.Errorf("failed resuming %s: %w", appName, err)

		return
	}

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

func waitUntilAppIsRunning(ctx context.Context, app *api.AppCompact, err *error) {
	client := client.FromContext(ctx).API()

	for err == nil && app.Status != "running" {
		app, *err = client.GetAppCompact(ctx, app.Name)

		pause.For(ctx, time.Millisecond*100)
	}
}
