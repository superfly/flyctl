// Package version implements the version command chain.
package version

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
)

const saveInstallName = "saveinstall"

// New initializes and returns a new version Command.
func New() *cobra.Command {
	const (
		short = "Show version information for the flyctl command"

		long = `Shows version information for the flyctl command itself, including version
number and build date.`
	)

	version := command.New("version", short, long, run)

	// TODO: remove once installer is updated to use init-state
	flag.Add(version,
		flag.String{
			Name:        saveInstallName,
			Shorthand:   "s",
			Description: "Save parameter in config",
		},
	)

	version.AddCommand(
		newInitState(),
		newUpdate(),
	)

	return version
}

func run(ctx context.Context) (err error) {
	if saveInstall := flag.GetString(ctx, saveInstallName); saveInstall != "" {
		err = executeInitState(
			iostreams.FromContext(ctx),
			cache.FromContext(ctx),
			saveInstall,
		)

		return
	}

	var (
		cfg  = config.FromContext(ctx)
		info = buildinfo.Info()
		out  = iostreams.FromContext(ctx).Out
	)

	if cfg.JSONOutput {
		err = json.NewEncoder(out).Encode(info)
	} else {
		_, err = fmt.Fprintln(out, info)
	}

	return
}
