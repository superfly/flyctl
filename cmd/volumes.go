package cmd

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"

	"github.com/superfly/flyctl/docstrings"
)

func newVolumesCommand() *Command {
	volumesStrings := docstrings.Get("volumes")
	volumesCmd := BuildCommandKS(nil, nil, volumesStrings, os.Stdout, requireAppName, requireSession)
	volumesCmd.Aliases = []string{"vol"}

	listStrings := docstrings.Get("volumes.list")
	BuildCommandKS(volumesCmd, runListVolumes, listStrings, os.Stdout, requireAppName, requireSession)

	createStrings := docstrings.Get("volumes.create")
	createCmd := BuildCommandKS(volumesCmd, runCreateVolume, createStrings, os.Stdout, requireAppName, requireSession)
	createCmd.Args = cobra.ExactArgs(1)

	createCmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Description: "Set region for new volume",
	})

	createCmd.AddIntFlag(IntFlagOpts{
		Name:        "size",
		Description: "Size of volume in gigabytes, default 10GB",
		Default:     10,
	})

	createCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "encrypted",
		Description: "Encrypt the volume (default: true)",
		Default:     true,
	})

	deleteStrings := docstrings.Get("volumes.delete")
	deleteCmd := BuildCommandKS(volumesCmd, runDestroyVolume, deleteStrings, os.Stdout, requireSession)
	deleteCmd.Args = cobra.ExactArgs(1)

	showStrings := docstrings.Get("volumes.show")
	showCmd := BuildCommandKS(volumesCmd, runShowVolume, showStrings, os.Stdout, requireSession)
	showCmd.Args = cobra.ExactArgs(1)

	return volumesCmd
}

func runListVolumes(ctx *cmdctx.CmdContext) error {

	volumes, err := ctx.Client.API().GetVolumes(ctx.AppName)

	if err != nil {
		return err
	}

	if len(volumes) == 0 {
		fmt.Printf("No Volumes Defined for %s\n", ctx.AppName)
		return nil
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(volumes)
		return nil
	}

	table := helpers.MakeSimpleTable(ctx.Out, []string{"ID", "Name", "Size", "Region", "Created At"})

	for _, v := range volumes {
		table.Append([]string{v.ID, v.Name, strconv.Itoa(v.SizeGb) + "GB", v.Region, humanize.Time(v.CreatedAt)})
	}

	table.Render()

	return nil
}

func runCreateVolume(ctx *cmdctx.CmdContext) error {

	volName := ctx.Args[0]

	region, err := ctx.Config.GetString("region")

	if err != nil {
		return err
	}

	app, err := ctx.Client.API().GetApp(ctx.AppName)

	if err != nil {
		return err
	}

	appid := app.ID

	if region == "" {
		return fmt.Errorf("--region <region> flag required")
	}

	sizeGb := ctx.Config.GetInt("size")

	volume, err := ctx.Client.API().CreateVolume(appid, volName, region, sizeGb, ctx.Config.GetBool("encrypted"))

	if err != nil {
		return err
	}

	fmt.Printf("%10s: %s\n", "ID", volume.ID)
	fmt.Printf("%10s: %s\n", "Name", volume.Name)
	fmt.Printf("%10s: %s\n", "Region", volume.Region)
	fmt.Printf("%10s: %d\n", "Size GB", volume.SizeGb)
	fmt.Printf("%10s: %t\n", "Encrypted", volume.Encrypted)
	fmt.Printf("%10s: %s\n", "Created at", volume.CreatedAt.Format(time.RFC822))

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

func runShowVolume(ctx *cmdctx.CmdContext) error {
	volID := ctx.Args[0]

	volume, err := ctx.Client.API().GetVolume(volID)

	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(volume)
		return nil
	}

	fmt.Printf("%10s: %s\n", "ID", volume.ID)
	fmt.Printf("%10s: %s\n", "Name", volume.Name)
	fmt.Printf("%10s: %s\n", "Region", volume.Region)
	fmt.Printf("%10s: %d\n", "Size GB", volume.SizeGb)
	fmt.Printf("%10s: %s\n", "Created at", volume.CreatedAt.Format(time.RFC822))

	return nil
}
