// Package version implements the version command chain.
package version

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
)

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
			Name:        "saveinstall",
			Shorthand:   "s",
			Description: "Save parameter in config",
			Hidden:      true,
		},
	)

	version.AddCommand(
		newSaveInstall(),
		newUpgrade(),
	)

	flag.Add(version, flag.JSONOutput())
	return version
}

func run(ctx context.Context) (err error) {
	if channel := flag.GetString(ctx, "saveinstall"); channel != "" {
		return saveInstall(ctx, channel, true)
	}

	var (
		cfg        = config.FromContext(ctx)
		info       = buildinfo.Info()
		simpleInfo = buildinfo.SimpleInfo(info)
		out        = iostreams.FromContext(ctx).Out
		verbose    = flag.GetBool(ctx, "verbose")
	)

	switch {
	case cfg.JSONOutput && verbose:
		err = json.NewEncoder(out).Encode(info)
	case cfg.JSONOutput && !verbose:
		err = json.NewEncoder(out).Encode(simpleInfo)
	case !cfg.JSONOutput && verbose:
		_, err = fmt.Fprintln(out, info)
	default:
		_, err = fmt.Fprintln(out, simpleInfo)
	}

	return
}
