package cmd

import (
	"fmt"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/flyctl"
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
	BuildCommand(cmd, runPlatformRegions, regionsStrings.Usage, regionsStrings.Short, regionsStrings.Long, os.Stdout, requireSession)

	vmSizesStrings := docstrings.Get("platform.vmsizes")
	BuildCommand(cmd, runPlatformVMSizes, vmSizesStrings.Usage, vmSizesStrings.Short, vmSizesStrings.Long, os.Stdout, requireSession)

	return cmd
}

func runPlatformRegions(ctx *cmdctx.CmdContext) error {
	regions, err := ctx.Client.API().PlatformRegions()
	if err != nil {
		return err
	}

	fmt.Println(ctx.GlobalConfig.GetBool(flyctl.ConfigJSONOutput))

	return ctx.Frender(ctx.Out, cmdctx.PresenterOption{
		Presentable: &presenters.Regions{Regions: regions},
	})
}

func runPlatformVMSizes(ctx *cmdctx.CmdContext) error {
	sizes, err := ctx.Client.API().PlatformVMSizes()
	if err != nil {
		return err
	}

	return ctx.Frender(ctx.Out, cmdctx.PresenterOption{
		Presentable: &presenters.VMSizes{VMSizes: sizes},
	})
}
