package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/client"

	"github.com/superfly/flyctl/docstrings"
)

func newVolumesCommand(client *client.Client) *Command {
	volumesStrings := docstrings.Get("volumes")
	volumesCmd := BuildCommandKS(nil, nil, volumesStrings, client, requireAppName, requireSession)
	volumesCmd.Aliases = []string{"vol"}

	listStrings := docstrings.Get("volumes.list")
	BuildCommandKS(volumesCmd, runListVolumes, listStrings, client, requireAppName, requireSession)

	createStrings := docstrings.Get("volumes.create")
	createCmd := BuildCommandKS(volumesCmd, runCreateVolume, createStrings, client, requireAppName, requireSession)
	createCmd.Args = cobra.ExactArgs(1)

	createCmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Description: "Set region for new volume",
	})

	createCmd.AddIntFlag(IntFlagOpts{
		Name:        "size",
		Description: "Size of volume in gigabytes",
		Default:     10,
	})

	createCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "encrypted",
		Description: "Encrypt the volume",
		Default:     true,
	})

	deleteStrings := docstrings.Get("volumes.delete")
	deleteCmd := BuildCommandKS(volumesCmd, runDeleteVolume, deleteStrings, client, requireSession)
	deleteCmd.Args = cobra.ExactArgs(1)
	deleteCmd.AddBoolFlag(BoolFlagOpts{Name: "yes", Shorthand: "y", Description: "Accept all confirmations"})

	showStrings := docstrings.Get("volumes.show")
	showCmd := BuildCommandKS(volumesCmd, runShowVolume, showStrings, client, requireSession)
	showCmd.Args = cobra.ExactArgs(1)

	snapshotStrings := docstrings.Get("volumes.snapshots")
	snapshotCmd := BuildCommandKS(volumesCmd, nil, snapshotStrings, client, requireSession)

	snapshotListStrings := docstrings.Get("volumes.snapshots.list")
	snapshotListCmd := BuildCommandKS(snapshotCmd, runListVolumeSnapshots, snapshotListStrings, client, requireSession)
	snapshotListCmd.Args = cobra.ExactArgs(1)

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

	table := helpers.MakeSimpleTable(ctx.Out, []string{"ID", "Name", "Size", "Region", "Attached VM", "Created At"})

	for _, v := range volumes {
		var attachedAllocID string
		if v.AttachedAllocation != nil {
			attachedAllocID = v.AttachedAllocation.IDShort
			if v.AttachedAllocation.TaskName != "app" {
				attachedAllocID = fmt.Sprintf("%s (%s)", v.AttachedAllocation.IDShort, v.AttachedAllocation.TaskName)
			}
		}
		table.Append([]string{v.ID, v.Name, strconv.Itoa(v.SizeGb) + "GB", v.Region, attachedAllocID, humanize.Time(v.CreatedAt)})
	}

	table.Render()

	return nil
}

func runCreateVolume(ctx *cmdctx.CmdContext) error {

	volName := ctx.Args[0]

	region := ctx.Config.GetString("region")

	app, err := ctx.Client.API().GetApp(ctx.AppName)

	if err != nil {
		return err
	}

	appid := app.ID

	if region == "" {
		return fmt.Errorf("--region <region> flag required")
	}

	sizeGb := ctx.Config.GetInt("size")

	input := api.CreateVolumeInput{
		AppID:     appid,
		Name:      volName,
		Region:    region,
		SizeGb:    sizeGb,
		Encrypted: ctx.Config.GetBool("encrypted"),
	}

	volume, err := ctx.Client.API().CreateVolume(input)

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

func runDeleteVolume(ctx *cmdctx.CmdContext) error {

	volID := ctx.Args[0]

	if !ctx.Config.GetBool("yes") {
		fmt.Println(aurora.Red("Deleting a volume is not reversible."))

		confirm := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf("Delete volume %s?", volID),
		}
		err := survey.AskOne(prompt, &confirm)

		if err != nil {
			return err
		}

		if !confirm {
			return nil
		}
	}

	data, err := ctx.Client.API().DeleteVolume(volID)

	if err != nil {
		return err
	}

	fmt.Printf("Deleted volume %s from %s\n", volID, data.Name)

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
	fmt.Printf("%10s: %t\n", "Encrypted", volume.Encrypted)
	fmt.Printf("%10s: %s\n", "Created at", volume.CreatedAt.Format(time.RFC822))

	return nil
}

func runListVolumeSnapshots(ctx *cmdctx.CmdContext) error {
	volName := ctx.Args[0]

	snapshots, err := ctx.Client.API().GetVolumeSnapshots(volName)
	if err != nil {
		return err
	}

	if len(snapshots) == 0 {
		fmt.Printf("No snapshots available for volume %q\n", volName)
		return nil
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(snapshots)
		return nil
	}

	table := helpers.MakeSimpleTable(ctx.Out, []string{"id", "size", "created at"})

	// Sort snapshots from newest to oldest
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	for _, s := range snapshots {
		size, _ := strconv.Atoi(s.Size)
		table.Append([]string{s.ID, helpers.BytesToHumanReadable(size, 2), humanize.Time(s.CreatedAt)})
	}

	table.Render()

	return nil
}
