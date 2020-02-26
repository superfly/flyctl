package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/docstrings"

	"github.com/superfly/flyctl/cmd/presenters"
)

func newPlatformCommand() *Command {
	platformStrings := docstrings.Get("platform")

	cmd := &Command{
		Command: &cobra.Command{
			Use:   platformStrings.Usage,
			Short: platformStrings.Short,
			Long:  platformStrings.Long,
		},
	}
	regionsStrings := docstrings.Get("platform.regions")
	BuildCommand(cmd, runPlatformRegions, regionsStrings.Usage, regionsStrings.Short, regionsStrings.Long, true, os.Stdout)

	return cmd
}

func runPlatformRegions(ctx *CmdContext) error {
	regions, err := ctx.FlyClient.PlatformRegions()
	if err != nil {
		return err
	}

	return ctx.RenderView(PresenterOption{
		Presentable: &presenters.Regions{Regions: regions},
	})
}
