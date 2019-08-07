package cmd

import (
	"os"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newAppListCommand() *Command {
	return BuildCommand(runAppsList, "apps", "list apps", os.Stdout, true)
}

func runAppsList(ctx *CmdContext) error {
	query := `
		query {
			apps {
				nodes {
					id
					name
					organization {
						slug
					}
					runtime
				}
			}
		}
		`

	req := ctx.FlyClient.NewRequest(query)

	data, err := ctx.FlyClient.Run(req)
	if err != nil {
		return err
	}

	return ctx.Render(&presenters.AppsPresenter{Apps: data.Apps.Nodes})
}
