// Package root implements the root command.
package root

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/apps"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/version"
)

// New initializes and returns a reference to a new root command.
func New() *cobra.Command {
	root := command.New("flyctl", nil)
	root.SilenceUsage = true
	root.SilenceErrors = true

	fs := root.PersistentFlags()

	_ = fs.StringP(flag.AccessToken, "t", "", "Fly API Access Token")
	_ = fs.BoolP(flag.JSON, "j", false, "JSON output")
	_ = fs.BoolP(flag.Verbose, "v", false, "Verbose output")

	root.AddCommand(
		apps.New(),
		version.New(),
	)

	return root
}
