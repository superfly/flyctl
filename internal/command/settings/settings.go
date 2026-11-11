package settings

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const long = `View and change local settings for fly on this computer: anonymized
analytics, automatic updates, and the synthetics agent. Settings are saved
in your fly config file.

Run a setting's command, such as ` + "`fly settings autoupdate`" + `, to see its
current value, and add ` + "`enable`" + ` or ` + "`disable`" + ` to change it.`

	cmd := command.New("settings", "Manage flyctl settings", long, nil)

	cmd.AddCommand(
		newAnalytics(),
		newAutoUpdate(),
		newSynthetics(),
	)

	return cmd
}
