package logs

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newShipper() *cobra.Command {

	const (
		short = "Manage a VM that ships logs to an external provider"
		long  = short + "\n"
	)

	cmd := command.New("shipper", short, long, nil)

	cmd.AddCommand(newShipperSetup())

	return cmd
}
