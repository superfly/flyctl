package ips

import (
	"github.com/superfly/flyctl/internal/command"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		long = `Allocate, list and release the IP addresses of an app. Public (Anycast)
IPv4 and IPv6 addresses route internet traffic to the app's services;
egress IPs give Machines a static outgoing address. Start with
` + "`fly ips list`" + ` and ` + "`fly ips allocate`" + `.`
		short = `Manage IP addresses for apps`
	)

	cmd := command.New("ips", short, long, nil)
	cmd.Aliases = []string{"ip"}
	cmd.AddCommand(
		newList(),
		newAllocate(),
		newAllocatev4(),
		newAllocatev6(),
		newAllocateEgress(),
		newPrivate(),
		newRelease(),
		newReleaseEgress(),
	)

	return cmd
}
