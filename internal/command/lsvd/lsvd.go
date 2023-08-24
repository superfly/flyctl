package lsvd

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	cmd := command.New("lsvd", "", "", nil)
	cmd.Hidden = true
	cmd.AddCommand(newSetup())
	return cmd
}
