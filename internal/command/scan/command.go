package scan

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		usage = "scan"
		short = "Scan machine images for vulnerabilities or to get an SBOM"
		long  = "Scan machine images for vulnerabilities or to get an SBOM."
	)
	cmd := command.New(usage, short, long, nil)

	cmd.AddCommand(
		newSbom(),
		newVulns(),
		newVulnSummary(),
	)

	return cmd
}
