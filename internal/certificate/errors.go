package certificate

import (
	"fmt"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/iostreams"
)

// DisplayValidationErrors shows certificate validation errors in a user-friendly format
func DisplayValidationErrors(io *iostreams.IOStreams, errors []fly.AppCertificateValidationError) {
	if len(errors) == 0 {
		return
	}

	cs := io.ColorScheme()

	fmt.Fprintf(io.Out, "\n%s\n", cs.Yellow("Certificate validation issues:"))

	for _, err := range errors {
		fmt.Fprintf(io.Out, "\n  %s\n", err.Message)
		if err.Remediation != "" {
			fmt.Fprintf(io.Out, "  %s %s\n", cs.Bold("Fix:"), err.Remediation)
		}
		fmt.Fprintf(io.Out, "  %s\n",
			cs.Gray("Checked "+format.RelativeTime(err.Timestamp)))
	}
}
