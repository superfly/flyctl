package mysql

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command/extensions/planetscale"
)

func New() (cmd *cobra.Command) {
	return planetscale.New()
}
