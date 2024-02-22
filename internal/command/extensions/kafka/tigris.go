package kafka

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func New() (cmd *cobra.Command) {
	const (
		short = "Provision and manage Upstash Kafka clusters"
		long  = short + "\n"
	)

	cmd = command.New("kafka", short, long, nil)
	cmd.AddCommand(create(), update(), list(), dashboard(), destroy(), status())

	return cmd
}

var SharedFlags = flag.Set{}
