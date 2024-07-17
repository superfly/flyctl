package registry

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		usage = "registry"
		short = "Operate on registry images"
		long  = "Scan registry images for an SBOM or vulnerabilities."
	)
	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newSbom(),
		newVulns(),
		newVulnSummary(),
	)

	return cmd
}
