// Package version implements the version command chain.
package version

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
)

// New initializes and returns a new version Command.
func New() *cobra.Command {
	version := newVersion()

	version.AddCommand(
		newUpdate(),
		newInitState(),
	)

	return version
}

func newVersion() *cobra.Command {
	version := command.FromDocstrings("version", run)

	flag.Add(version, nil,
		flag.String{
			Name:        "saveinstall",
			Shorthand:   "s",
			Description: "Save parameter in config",
			Hidden:      true,
		},
	)

	return version
}

func run(ctx context.Context) error {
	var (
		out  = iostreams.FromContext(ctx).Out
		info = buildinfo.Info()
	)

	if config.FromContext(ctx).JSONOutput {
		_ = json.NewEncoder(out).Encode(info)
	} else {
		fmt.Fprintln(out, info)
	}

	return nil
}
