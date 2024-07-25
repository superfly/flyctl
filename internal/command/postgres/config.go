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
	"max-wal-senders":            "max_wal_senders",
	"max-replication-slots":      "max_replication_slots",
	"max-connections":            "max_connections",
	"work-mem":                   "work_mem",
	"maintenance-work-mem":       "maintenance_work_mem",
	"shared-buffers":             "shared_buffers",
	"log-statement":              "log_statement",
	"log-min-duration-statement": "log_min_duration_statement",
	"shared-preload-libraries":   "shared_preload_libraries",
}

func newConfig() (cmd *cobra.Command) {
	const (
		short = "Show and manage Postgres configuration."
		long  = short + "\n"
	)

	cmd = command.New("config", short, long, nil)

	cmd.AddCommand(
		newConfigShow(),
		newConfigUpdate(),
	)

	return
}
