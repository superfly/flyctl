package registry

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		usage = "registry"
		short = "Operate on registry images [experimental]"
		long  = "Scan registry images for an SBOM or vulnerabilities. These commands\n" +
			"are experimental and subject to change."
	)
	cmd := command.New(usage, short, long, nil)
	cmd.Hidden = true

	cmd.AddCommand(
		newFiles(),
		newSbom(),
		newVulns(),
		newVulnSummary(),
	)

	return cmd
}
