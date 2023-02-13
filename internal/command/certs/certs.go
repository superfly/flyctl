package certs

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		long = `Manages the certificates associated with a deployed application.
		Certificates are created by associating a hostname/domain with the application.
		When Fly is then able to validate that hostname/domain, the platform gets
		certificates issued for the hostname/domain by Let's Encrypt.`

		short = "Manages certificates"

		usage = "certs"
	)

	certs := command.New(usage, short, long, nil,
		command.RequireAppName, command.RequireSession)

	certs.AddCommand(
		newList(),
		newAdd(),
		newRemove(),
		newShow(),
		newCheck(),
	)

	return certs
}
