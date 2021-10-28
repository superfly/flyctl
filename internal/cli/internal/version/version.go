// Package version implements the version command chain.
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/update"
)

const saveInstallName = "saveInstall"

// New initializes and returns a new version Command.
func New() *cobra.Command {
	const (
		short = "Show version information for the flyctl command"

		long = `Shows version information for the flyctl command itself, including version
number and build date.`
	)

	version := command.New("version", short, long, run)

	flag.Add(version, nil,
		flag.String{
			Name:        saveInstallName,
			Shorthand:   "s",
			Description: "Save parameter in config",
			Hidden:      true,
		},
	)

	version.AddCommand(
		newUpdate(),
		newInitState(),
	)

	return version
}

func run(ctx context.Context) (err error) {
	if saveInstall := flag.GetString(ctx, saveInstallName); saveInstall != "" {
		path := filepath.Join(state.ConfigDirectory(ctx), "state.yml")

		err = update.InitState(path, saveInstall)

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
