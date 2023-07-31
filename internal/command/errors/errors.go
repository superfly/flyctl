package errors

import (
	"github.com/spf13/cobra"
	sentry_ext "github.com/superfly/flyctl/internal/command/extensions/sentry"
)

func New() (cmd *cobra.Command) {
	return sentry_ext.Dashboard()
}
