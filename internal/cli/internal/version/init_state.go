package version

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/cache"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/pkg/iostreams"
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
	channel := flag.Args(ctx)[0]
	cache.FromContext(ctx).SetChannel(channel)

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.ErrOut, "set channel to %s\n", channel)

	return nil
}
