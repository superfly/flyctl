// Package version implements the version command chain.
package version

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cli/internal/cmd"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/pkg/iostreams"
)

// New initializes and returns a new apps Command.
func New() *cobra.Command {
	version := newVersion()

	version.AddCommand(
		newUpdate(),
		newInitState(),
	)

	return version
}

func newVersion() *cobra.Command {
	version := cmd.New("version", run)

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

	if flag.GetJSONOutput(ctx) {
		_ = json.NewEncoder(out).Encode(info)
	} else {
		fmt.Fprintln(out, info)
	}

	return nil
}

func newUpdate() *cobra.Command {
	return cmd.New("version.update", runUpdate)
}

func runUpdate(context.Context) error {
	return nil
}

func newInitState() *cobra.Command {
	initState := cmd.Build(
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
