package postgres

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

// pgSettings maps the command-line argument to the actual pgParameter.
// This also acts as a whitelist as far as what's configurable via flyctl and
// can be expanded on as needed.
var pgSettings = map[string]string{
	"wal-level":                  "wal_level",
	"max-connections":            "max_connections",
	"shared-buffers":             "shared_buffers",
	"log-statement":              "log_statement",
	"log-min-duration-statement": "log_min_duration_statement",
	"shared-preload-libraries":   "shared_preload_libraries",
}

func newConfig() (cmd *cobra.Command) {
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
