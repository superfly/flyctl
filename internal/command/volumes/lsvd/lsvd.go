package lsvd

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const help = "Manage log-structured virtual disks (LSVD) on an app"
	cmd := command.New("lsvd", help, help, nil)
	cmd.Hidden = true
	cmd.AddCommand(newSetup())
	return cmd
}
