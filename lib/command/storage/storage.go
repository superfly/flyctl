package storage

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/lib/command/extensions/tigris"
)

func New() (cmd *cobra.Command) {
	return tigris.New()
}
