package version

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/update"
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

func runInitState(ctx context.Context) error {
	path := filepath.Join(state.ConfigDirectory(ctx), "state.yml")

	return update.InitState(path, flag.Args(ctx)[0])
}
