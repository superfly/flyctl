package ips

import (
	"github.com/superfly/flyctl/internal/command"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		long  = `Commands for managing IP addresses associated with an application`
		short = `Manage IP addresses for apps`
	)

	cmd := command.New("ips", short, long, nil)
	cmd.AddCommand(
		newList(),
		newAllocatev4(),
		newAllocatev6(),
		newPrivate(),
		newRelease(),
	)
	return cmd
}
