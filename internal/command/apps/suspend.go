package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func newSuspend() *cobra.Command {
	const (
		long  = `This command is deprecated. You may still use 'fly resume' to restore suspended apps.`
		short = "Suspend an application (deprecated)"
		usage = "suspend"
	)

	suspend := command.New(usage, short, long, RunSuspend,
		command.RequireSession)
	suspend.Hidden = true
	return suspend
}

// TODO: make internal once the suspend package is removed
func RunSuspend(ctx context.Context) (err error) {

	return fmt.Errorf("this command is deprecated. You may still resume suspended apps using 'fly resume'. Use 'fly scale count 0' if you need to stop an app temporarily\n")
}
