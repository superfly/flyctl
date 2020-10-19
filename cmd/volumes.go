package cmd

import (
	"fmt"
	"os"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"

	"github.com/superfly/flyctl/docstrings"
)

func newVolumesCommand() *Command {
	volumesStrings := /*docstrings.Get("volumes")*/ docstrings.KeyStrings{Usage: "volumes", Short: "Managing volumes", Long: ""}
	volumesCmd := BuildCommandKS(nil, nil, volumesStrings, os.Stdout, requireAppName, requireSession)

	listStrings := /* docstrings.Get("volumes.list") */ docstrings.KeyStrings{Usage: "list", Short: "List volumes", Long: ""}
	BuildCommandKS(volumesCmd, runListVolumes, listStrings, os.Stdout, requireAppName, requireSession)

	createStrings := /* docstrings.Get("volumes.create") */ docstrings.KeyStrings{Usage: "create <name>", Short: "Create volume", Long: ""}
	createCmd := BuildCommandKS(volumesCmd, runCreateVolume, createStrings, os.Stdout, requireAppName, requireSession)
	createCmd.Args = cobra.ExactArgs(1)

	createCmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Description: "Set region for new volume",
	})

	createCmd.AddIntFlag(IntFlagOpts{
		Name:        "size",
		Description: "Size of volume in gigabytes, default 5",
		Default:     5,
	})

	deleteStrings := /* docstrings.Get("volumes	delete") */ docstrings.KeyStrings{Usage: "delete <id>", Short: "Delete volume", Long: ""}
	deleteCmd := BuildCommandKS(volumesCmd, runDestroyVolume, deleteStrings, os.Stdout, requireSession)
	deleteCmd.Args = cobra.ExactArgs(1)

	return volumesCmd
}

func runListVolumes(ctx *cmdctx.CmdContext) error {

	volumes, err := ctx.Client.API().GetVolumes(ctx.AppName)

	if err != nil {
		return err
	}

	if len(volumes) == 0 {
		fmt.Println("No Volumes Defined")
		return nil
	}

	fmt.Printf("%-20s %-20s %-7s %-6s %-20s\n", "ID", "Name", "Size GB", "Region", "Created At")
	for _, n := range volumes {
		fmt.Printf("%-20s %-20s %-7d %-6s %-20s\n", n.ID, n.Name, n.SizeGb, n.Region, humanize.Time(n.CreatedAt))
	}

	return nil
}

func runCreateVolume(ctx *cmdctx.CmdContext) error {

	volName := ctx.Args[0]

	region, err := ctx.Config.GetString("region")

	if err != nil {
		return err
	}

	app, err := ctx.Client.API().GetApp(ctx.AppName)
	appid := app.ID

	if region == "" {
		return fmt.Errorf("--region <region> flag required")
	}

	sizeGb := ctx.Config.GetInt("size")

	volume, err := ctx.Client.API().CreateVolume(appid, volName, region, sizeGb)

	if err != nil {
		return err
	}

	fmt.Printf("%10s: %s\n", "ID", volume.ID)
	fmt.Printf("%10s: %s\n", "Name", volume.Name)
	fmt.Printf("%10s: %s\n", "Region", volume.Region)
	fmt.Printf("%10s: %d\n", "Size GB", volume.SizeGb)

	return nil
}

func runDestroyVolume(ctx *cmdctx.CmdContext) error {

	volID := ctx.Args[0]

	data, err := ctx.Client.API().DeleteVolume(volID)

	if err != nil {
		return err
	}

	fmt.Printf("Destroyed volume %s from %s\n", volID, data.Name)

	return nil
}
