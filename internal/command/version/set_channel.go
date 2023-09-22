package version

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/iostreams"
)

func newChannel() *cobra.Command {
	const (
		short = "Set the update channel for flyctl"

		long = `Set the update channel for flyctl and installs the latest version on that channel. Valid channels are "stable", 
		"nightly", and "prNNNN" for unstable pull request builds.`
	)

	cmd := command.New("set-channel <channel>", short, long, runSetChannel)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runSetChannel(ctx context.Context) error {
	channel := flag.FirstArg(ctx)
	fmt.Println("change channel to", channel)

	cache.FromContext(ctx).SetChannel(channel)

	io := iostreams.FromContext(ctx)

	update.InstallInPlace(ctx, io, channel, false)

	return nil
}
