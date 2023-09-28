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

func newInstall() *cobra.Command {
	const (
		short = "Install another version of flyctl"

		long = `Install a specific version of flyctl, or install the latest version from a different update channel. Valid channels are "stable",
		"nightly", and "prNNNN" for unstable pull request builds.`
	)

	cmd := command.New("install <channel>", short, long, runInstall)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func runInstall(ctx context.Context) error {
	channel := flag.FirstArg(ctx)
	fmt.Println("changed channel to", channel)

	cache.FromContext(ctx).SetChannel(channel)

	io := iostreams.FromContext(ctx)

	update.InstallInPlace(ctx, io, channel, false)

	return nil
}
