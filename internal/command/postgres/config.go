package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newConfig() (cmd *cobra.Command) {
	// TODO - Add better top level docs.
	const (
		short = "View and manage Postgres configuration."
		long  = short + "\n"
	)

	cmd = command.New("config", short, long, nil)

	cmd.AddCommand(
		newConfigView(),
		newConfigUpdate(),
	)

	return
}

func newConfigView() (cmd *cobra.Command) {
	const (
		long = `View your Postgres configuration
`
		short = "View your Postgres configuration"
		usage = "view"
	)

	cmd = command.New(usage, short, long, nil,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return
}

func newConfigUpdate() (cmd *cobra.Command) {
	const (
		long = `Update Postgres configuration.
`
		short = "Update Postgres configuration."
		usage = "update"
	)

	cmd = command.New(usage, short, long, nil,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "max-connections",
			Description: "Sets the maximum number of concurrent connections.",
		},
		flag.String{
			Name:        "wal-level",
			Description: "Sets the level of information written to the WAL. (minimal, replica, logical).",
		},
		flag.String{
			Name:        "log-statement",
			Description: "Sets the type of statements logged. (none, ddl, mod, all)",
		},
		flag.String{
			Name:        "log-min-duration-statement",
			Description: "Sets the minimum execution time above which all statements will be logged. (ms)",
		},
		flag.Bool{
			Name:        "auto-confirm",
			Description: "Will automatically confirm changes without an interactive prompt.",
		},
	)

	return
}
