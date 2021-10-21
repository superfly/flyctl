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
	"github.com/superfly/flyctl/internal/cli/internal/flag"
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
	version := command.New("version", run)

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

	if false {
		_ = json.NewEncoder(out).Encode(info)
	} else {
		fmt.Fprintln(out, info)
	}

	return nil
}

func newUpdate() *cobra.Command {
	return command.New("version.update", runUpdate)
}

func runUpdate(context.Context) error {
	return nil
}

func newInitState() *cobra.Command {
	initState := command.Build(
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
