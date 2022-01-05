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

func newSuspend() *cobra.Command {
	const (
		long = `The APPS SUSPEND command will suspend an application. 
All instances will be halted leaving the application running nowhere.
It will continue to consume networking resources (IP address). See APPS RESUME
for details on restarting it.
`
		short = "Suspend an application"
		usage = "suspend [APPNAME]"
	)

	suspend := command.New(usage, short, long, RunSuspend,
		command.RequireSession)

	suspend.Args = cobra.ExactArgs(1)

	return suspend
}

// TODO: make internal once the suspend package is removed
func RunSuspend(ctx context.Context) (err error) {
	appName := flag.FirstArg(ctx)

	client := client.FromContext(ctx).API()
	if _, err = client.SuspendApp(ctx, appName); err != nil {
		err = fmt.Errorf("failed suspending %s: %w", appName, err)

		return
	}

	var status *api.AppStatus
	if status, err = client.GetAppStatus(ctx, appName, false); err != nil {
		err = fmt.Errorf("failed retrieving %s status: %w", status.Name, err)

		return
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out, "%s is now %s\n", status.Name, status.Status)

	allocs := len(status.Allocations)

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = io.ErrOut
	s.Prefix = fmt.Sprintf("Suspending %s with %d instances to stop ",
		status.Name, allocs)

	s.Start()

	for allocs > 0 {
		plural := ""
		if allocs > 1 {
			plural = "s"
		}

		s.Stop()
		s.Prefix = fmt.Sprintf("Suspending %s with %d instance%s to stop ", status.Name, allocs, plural)
		s.Restart()

		pause.For(ctx, time.Millisecond*100)

		if status, err = client.GetAppStatus(ctx, appName, false); err != nil {
			s.Stop()

			err = fmt.Errorf("failed retrieving %s status: %w", appName, err)

			return
		}

		allocs = len(status.Allocations)
	}

	s.FinalMSG = fmt.Sprintf("Suspend complete - %s is now suspended with no running instances\n", status.Name)
	s.Stop()

	return nil
}
