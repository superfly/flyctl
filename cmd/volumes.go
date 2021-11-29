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

	createCmd.AddBoolFlag(BoolFlagOpts{
		Name:        "require-unique-zone",
		Description: "Require volume to be placed in separate hardware zone from existing volumes",
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

func runListVolumes(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	volumes, err := cmdCtx.Client.API().GetVolumes(ctx, cmdCtx.AppName)

	if err != nil {
		return err
	}

	if len(volumes) == 0 {
		fmt.Printf("No Volumes Defined for %s\n", cmdCtx.AppName)
		return nil
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(volumes)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"ID", "Name", "Size", "Region", "Zone", "Attached VM", "Created At"})

	for _, v := range volumes {
		var attachedAllocID string
		if v.AttachedAllocation != nil {
			attachedAllocID = v.AttachedAllocation.IDShort
			if v.AttachedAllocation.TaskName != "app" {
				attachedAllocID = fmt.Sprintf("%s (%s)", v.AttachedAllocation.IDShort, v.AttachedAllocation.TaskName)
			}
		}
		table.Append([]string{v.ID, v.Name, strconv.Itoa(v.SizeGb) + "GB", v.Region, v.Host.ID, attachedAllocID, humanize.Time(v.CreatedAt)})
	}

	table.Render()

	return nil
}

func runCreateVolume(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	volName := cmdCtx.Args[0]

	region := cmdCtx.Config.GetString("region")

	app, err := cmdCtx.Client.API().GetApp(ctx, cmdCtx.AppName)

	if err != nil {
		return err
	}

	appid := app.ID

	if region == "" {
		return fmt.Errorf("--region <region> flag required")
	}

	sizeGb := cmdCtx.Config.GetInt("size")

	input := api.CreateVolumeInput{
		AppID:             appid,
		Name:              volName,
		Region:            region,
		SizeGb:            sizeGb,
		Encrypted:         cmdCtx.Config.GetBool("encrypted"),
		RequireUniqueZone: cmdCtx.Config.GetBool("require-unique-zone"),
	}

	volume, err := cmdCtx.Client.API().CreateVolume(ctx, input)

	if err != nil {
		return err
	}

	fmt.Printf("%10s: %s\n", "ID", volume.ID)
	fmt.Printf("%10s: %s\n", "Name", volume.Name)
	fmt.Printf("%10s: %s\n", "Region", volume.Region)
	fmt.Printf("%10s: %s\n", "Zone", volume.Host.ID)
	fmt.Printf("%10s: %d\n", "Size GB", volume.SizeGb)
	fmt.Printf("%10s: %t\n", "Encrypted", volume.Encrypted)
	fmt.Printf("%10s: %s\n", "Created at", volume.CreatedAt.Format(time.RFC822))

	return nil
}

func runDeleteVolume(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	volID := cmdCtx.Args[0]

	if !cmdCtx.Config.GetBool("yes") {
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

	data, err := cmdCtx.Client.API().DeleteVolume(ctx, volID)

	if err != nil {
		return err
	}

	fmt.Printf("Deleted volume %s from %s\n", volID, data.Name)

	return nil
}

func runShowVolume(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	volID := cmdCtx.Args[0]

	volume, err := cmdCtx.Client.API().GetVolume(ctx, volID)

	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(volume)
		return nil
	}

	fmt.Printf("%10s: %s\n", "ID", volume.ID)
	fmt.Printf("%10s: %s\n", "Name", volume.Name)
	fmt.Printf("%10s: %s\n", "Region", volume.Region)
	fmt.Printf("%10s: %s\n", "Zone", volume.Host.ID)
	fmt.Printf("%10s: %d\n", "Size GB", volume.SizeGb)
	fmt.Printf("%10s: %t\n", "Encrypted", volume.Encrypted)
	fmt.Printf("%10s: %s\n", "Created at", volume.CreatedAt.Format(time.RFC822))

	return nil
}

func runListVolumeSnapshots(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	volName := cmdCtx.Args[0]

	snapshots, err := cmdCtx.Client.API().GetVolumeSnapshots(ctx, volName)
	if err != nil {
		return err
	}

	if len(snapshots) == 0 {
		fmt.Printf("No snapshots available for volume %q\n", volName)
		return nil
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(snapshots)
		return nil
	}

	table := helpers.MakeSimpleTable(cmdCtx.Out, []string{"id", "size", "created at"})

	// Sort snapshots from newest to oldest
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	for _, s := range snapshots {
		size, _ := strconv.Atoi(s.Size)
		table.Append([]string{s.ID, humanize.IBytes(uint64(size)), humanize.Time(s.CreatedAt)})
	}

	table.Render()

	return nil
}
