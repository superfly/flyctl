package tokens

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() *cobra.Command {
	const (
		short = "Manage Fly.io API tokens"
		long  = `Create, list, revoke and inspect Fly.io API tokens. Tokens created here
are limited in scope, for example to deploying one app or reading one
organization, and are meant for CI and other automation.

Start with ` + "`fly tokens create deploy`" + ` for an app deploy token, and use
` + "`fly tokens list`" + ` and ` + "`fly tokens revoke`" + ` to manage existing ones.`
		usage = "tokens"
	)

	cmd := command.New(usage, short, long, nil)

	hiddenDeploy := newDeploy()
	hiddenDeploy.Hidden = true

	hiddenOrg := newOrg()
	hiddenOrg.Hidden = true

	cmd.AddCommand(
		newCreate(),
		newList(),
		newRevoke(),
		newAttenuate(),
		newDebug(),
		new3P(),
		hiddenDeploy,
		hiddenOrg,
	)

	return cmd
}
