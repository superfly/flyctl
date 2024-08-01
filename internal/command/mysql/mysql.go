package mysql

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command/extensions/fly_mysql"
)

func New() (cmd *cobra.Command) {
	return fly_mysql.New()
}
