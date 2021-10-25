package version

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newInitState() *cobra.Command {
	initState := command.New(
		"init-state",
		"init-state",
		"Initialize installation state",
		runInitState)

	initState.Hidden = true

	initState.Args = cobra.ExactArgs(1)

	return initState
}

func runInitState(context.Context) error {
	return nil
}
