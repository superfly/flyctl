package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	surveyterminal "github.com/AlecAivazis/survey/v2/terminal"
	"github.com/dustin/go-humanize"
	"github.com/google/shlex"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/pkg/logs"
	"github.com/superfly/flyctl/terminal"
)

func newMachineCommand(client *client.Client) *Command {
	keystrings := docstrings.Get("machine")
	cmd := BuildCommandCobra(nil, nil, &cobra.Command{
		Use:     keystrings.Usage,
		Short:   keystrings.Short,
		Long:    keystrings.Long,
		Aliases: []string{"machines", "m"},
	}, client)

	newMachineRunCommand(cmd, client)
	newMachineListCommand(cmd, client)
	newMachineStopCommand(cmd, client)
	newMachineStartCommand(cmd, client)
	newMachineKillCommand(cmd, client)
	newMachineRemoveCommand(cmd, client)
	newMachineCloneCommand(cmd, client)

	return cmd
}

func newMachineListCommand(parent *Command, client *client.Client) {
	keystrings := docstrings.Get("machine.list")
	cmd := BuildCommandCobra(parent, runMachineList, &cobra.Command{
		Use:     keystrings.Usage,
		Short:   keystrings.Short,
		Long:    keystrings.Long,
		Aliases: []string{"ls"},
	}, client, requireSession, optionalAppName)

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "all",
		Description: "Show machines in all states",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "state",
		Default:     "started",
		Description: "List machines in a specific state",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "quiet",
		Shorthand:   "q",
		Description: "Only list machine ids",
	})
}

func runMachineList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	state := cmdCtx.Config.GetString("state")
	if cmdCtx.Config.GetBool("all") {
		state = ""
	}
	machines, err := cmdCtx.Client.API().ListMachines(ctx, cmdCtx.AppName, state)
	if err != nil {
		return errors.Wrap(err, "could not get list of machines")
	}

	if cmdCtx.Config.GetBool("quiet") {
		for _, machine := range machines {
			fmt.Println(machine.ID)
		}
		return nil
	}

	data := [][]string{}

	for _, machine := range machines {

		var ipv6 string

		for _, ip := range machine.IPs.Nodes {
			if ip.Family == "v6" && ip.Kind == "privatenet" {
				ipv6 = ip.IP
			}
		}

		row := []string{
			machine.ID,
			machine.Config.Image,
			machine.CreatedAt.String(),
			machine.State,
			machine.Region,
			machine.Name,
			ipv6,
		}
		if cmdCtx.AppName == "" {
			row = append(row, machine.App.Name)
		}
		data = append(data, row)
	}

	table := tablewriter.NewWriter(os.Stdout)
	headers := []string{"ID", "Image", "Created", "State", "Region", "Name", "IP Address"}
	if cmdCtx.AppName == "" {
		headers = append(headers, "App")
	}
	table.SetHeader(headers)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t") // pad with tabs
	table.SetNoWhiteSpace(true)
	table.AppendBulk(data) // Add Bulk Data
	table.Render()

	return nil
}

func newMachineStopCommand(parent *Command, client *client.Client) {
	cmd := BuildCommandKS(parent, runMachineStop, docstrings.Get("machine.stop"), client, requireSession, optionalAppName)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "signal",
		Shorthand:   "s",
		Description: "Signal to stop the machine with (default: SIGINT)",
	})

	cmd.AddIntFlag(IntFlagOpts{
		Name:        "time",
		Description: "Seconds to wait before killing the machine",
	})

	cmd.Args = cobra.MinimumNArgs(1)
}

func runMachineStop(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	for _, arg := range cmdCtx.Args {
		input := api.StopMachineInput{
			AppID:           cmdCtx.AppName,
			ID:              arg,
			Signal:          cmdCtx.Config.GetString("signal"),
			KillTimeoutSecs: cmdCtx.Config.GetInt("time"),
		}

		machine, err := cmdCtx.Client.API().StopMachine(ctx, input)
		if err != nil {
			return errors.Wrap(err, "could not stop machine")
		}

		fmt.Println(machine.ID)
	}

	return nil
}

func newMachineStartCommand(parent *Command, client *client.Client) {
	cmd := BuildCommandKS(parent, runMachineStart, docstrings.Get("machine.start"), client, requireSession, optionalAppName)

	cmd.Args = cobra.ExactArgs(1)
}

func runMachineStart(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	input := api.StartMachineInput{
		AppID: cmdCtx.AppName,
		ID:    cmdCtx.Args[0],
	}

	machine, err := cmdCtx.Client.API().StartMachine(ctx, input)
	if err != nil {
		return errors.Wrap(err, "could not stop machine")
	}

	fmt.Println(machine.ID)

	return nil
}

func newMachineKillCommand(parent *Command, client *client.Client) {
	cmd := BuildCommandKS(parent, runMachineKill, docstrings.Get("machine.kill"), client, requireSession, optionalAppName)

	cmd.Args = cobra.MinimumNArgs(1)
}

func runMachineKill(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	for _, arg := range cmdCtx.Args {
		input := api.KillMachineInput{
			AppID: cmdCtx.AppName,
			ID:    arg,
		}

		machine, err := cmdCtx.Client.API().KillMachine(ctx, input)
		if err != nil {
			return errors.Wrap(err, "could not stop machine")
		}

		fmt.Println(machine.ID)
	}

	return nil
}

func newMachineCloneCommand(parent *Command, client *client.Client) {
	keystrings := docstrings.Get("machine.clone")
	cmd := BuildCommandCobra(parent, runMachineClone, &cobra.Command{
		Use:     keystrings.Usage,
		Short:   keystrings.Short,
		Long:    keystrings.Long,
		Aliases: []string{"clone"},
	}, client, requireSession, requireAppName)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Shorthand:   "r",
		Description: "Target region",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Shorthand:   "n",
		Description: "The name of the machine",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "organization",
		Shorthand:   "o",
		Description: "Target organization",
	})
	cmd.Args = cobra.MinimumNArgs(1)
}

func runMachineClone(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	client := cmdCtx.Client.API()

	appName := cmdCtx.AppName
	machineID := cmdCtx.Args[0]
	name := cmdCtx.Config.GetString("name")

	regionCode := cmdCtx.Config.GetString("region")
	var region *api.Region
	region, err := selectRegion(ctx, client, regionCode)
	if err != nil {
		return err
	}

	orgCode := cmdCtx.Config.GetString("organization")
	org, err := selectOrganization(ctx, client, orgCode, nil)
	if err != nil {
		return err
	}

	// TODO - Add GetMachine endpoint so we don't have to query everything.
	machines, err := cmdCtx.Client.API().ListMachines(ctx, appName, "")
	if err != nil {
		return err
	}

	var machine *api.Machine
	for _, m := range machines {
		if m.ID == machineID {
			machine = m
			break
		}
	}

	if machine == nil {
		return fmt.Errorf("failed to resolve machine with id: %s", machineID)
	}

	if len(machine.Config.Mounts) > 0 {
		// This copies the existing Volume spec and just renames it.
		volumeHash, err := helpers.RandString(5)
		if err != nil {
			return err
		}

		mount := machine.Config.Mounts[0]
		mount.Volume = fmt.Sprintf("data_%s", volumeHash)
		machine.Config.Mounts = []api.MachineMount{mount}
	}

	input := api.LaunchMachineInput{
		AppID:   appName,
		Name:    name,
		OrgSlug: org.ID,
		Region:  region.Code,
		Config:  &machine.Config,
	}

	machine, app, err := client.LaunchMachine(ctx, input)
	if err != nil {
		return err
	}

	if cmdCtx.Config.GetBool("detach") {
		fmt.Println(machine.ID)
		return nil
	}

	opts := &logs.LogOptions{
		AppName: app.Name,
		VMID:    machine.ID,
	}

	stream, err := logs.NewNatsStream(ctx, client, opts)

	if err != nil {
		terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
		terminal.Debug("Falling back to log polling...")

		stream, err = logs.NewPollingStream(ctx, client, opts)
		if err != nil {
			return err
		}
	}

	presenter := presenters.LogPresenter{}
	entries := stream.Stream(ctx, opts)

	for {
		select {
		case <-ctx.Done():
			return stream.Err()
		case entry := <-entries:
			presenter.FPrint(cmdCtx.Out, cmdCtx.OutputJSON(), entry)
		}
	}
}

func newMachineRemoveCommand(parent *Command, client *client.Client) {
	keystrings := docstrings.Get("machine.remove")
	cmd := BuildCommandCobra(parent, runMachineRemove, &cobra.Command{
		Use:     keystrings.Usage,
		Short:   keystrings.Short,
		Long:    keystrings.Long,
		Aliases: []string{"rm"},
	}, client, requireSession, optionalAppName)

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "force",
		Shorthand:   "f",
		Description: "force kill machine if it's running",
	})

	cmd.Args = cobra.MinimumNArgs(1)
}

func runMachineRemove(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	for _, arg := range cmdCtx.Args {
		input := api.RemoveMachineInput{
			AppID: cmdCtx.AppName,
			ID:    arg,
			Kill:  cmdCtx.Config.GetBool("force"),
		}

		machine, err := cmdCtx.Client.API().RemoveMachine(ctx, input)
		if err != nil {
			return errors.Wrap(err, "could not stop machine")
		}

		fmt.Println(machine.ID)
	}

	return nil
}

func newMachineRunCommand(parent *Command, client *client.Client) {
	cmd := BuildCommandKS(parent, runMachineRun, docstrings.Get("machine.run"), client, requireSession, optionalAppName)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "id",
		Description: "Machine ID, is previously known",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Shorthand:   "n",
		Description: "Machine name, will be generated if missing",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `The organization that will own the app`,
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Shorthand:   "r",
		Description: "Region to deploy the machine to (see `flyctl platform regions`)",
	})

	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "port",
		Shorthand:   "p",
		Description: "Exposed port mappings (format: edgePort[:machinePort]/[protocol[:handler]])",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "size",
		Shorthand:   "s",
		Description: "Size of the machine",
	})

	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "env",
		Shorthand:   "e",
		Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	})

	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "volume",
		Shorthand:   "v",
		Description: "Volumes to mount in the form of <volume_id_or_name>:/path/inside/machine[:<options>]",
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "entrypoint",
		Description: "ENTRYPOINT replacement",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Shorthand:   "d",
		Description: "Detach from the machine's logs",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name: "build-only",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "build-remote-only",
		Description: "Perform builds remotely without using the local docker daemon",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "build-local-only",
		Description: "Only perform builds locally using the local docker daemon",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "dockerfile",
		Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
	})
	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "build-arg",
		Description: "Set of build time variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image-label",
		Description: "Image label to use when tagging and pushing to the fly registry. Defaults to \"deployment-{timestamp}\".",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "build-target",
		Description: "Set the target build stage to build if the Dockerfile has more than one stage",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "no-build-cache",
		Description: "Do not use the cache when building the image",
	})

	cmd.Command.Args = cobra.MinimumNArgs(1)
}

func runMachineRun(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	if cmdCtx.AppName == "" {
		confirm := false
		prompt := &survey.Confirm{
			Message: "Running a machine without specifying an app will create one for you, is this what you want?",
		}
		err := survey.AskOne(prompt, &confirm)
		if err != nil {
			if err == surveyterminal.InterruptErr {
				return nil
			}
			return err
		}

		if !confirm {
			return nil
		}
	}

	orgSlug := cmdCtx.Config.GetString("org")
	org, err := selectOrganization(ctx, cmdCtx.Client.API(), orgSlug, nil)
	if err != nil {
		return err
	}

	if cmdCtx.MachineConfig == nil {
		cmdCtx.MachineConfig = &api.MachineConfig{}
	}

	if extraEnv := cmdCtx.Config.GetStringSlice("env"); len(extraEnv) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("env"))
		if err != nil {
			return errors.Wrap(err, "invalid env")
		}
		cmdCtx.MachineConfig.Env = parsedEnv
	}

	machineConf := cmdCtx.MachineConfig

	var img *imgsrc.DeploymentImage

	daemonType := imgsrc.NewDockerDaemonType(!cmdCtx.Config.GetBool("build-remote-only"), !cmdCtx.Config.GetBool("build-local-only"))
	resolver := imgsrc.NewResolver(daemonType, cmdCtx.Client.API(), cmdCtx.AppName, cmdCtx.IO)

	imageOrPath := cmdCtx.Args[0]
	// build if relative or absolute path
	if strings.HasPrefix(imageOrPath, ".") || strings.HasPrefix(imageOrPath, "/") {
		opts := imgsrc.ImageOptions{
			AppName:    cmdCtx.AppName,
			WorkingDir: path.Join(cmdCtx.WorkingDir, imageOrPath),
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageLabel: cmdCtx.Config.GetString("image-label"),
			Target:     cmdCtx.Config.GetString("build-target"),
			NoCache:    cmdCtx.Config.GetBool("no-build-cache"),
		}
		if dockerfilePath := cmdCtx.Config.GetString("dockerfile"); dockerfilePath != "" {
			dockerfilePath, err := filepath.Abs(dockerfilePath)
			if err != nil {
				return err
			}
			opts.DockerfilePath = dockerfilePath
		}

		extraArgs, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("build-arg"))
		if err != nil {
			return errors.Wrap(err, "invalid build-arg")
		}
		opts.ExtraBuildArgs = extraArgs

		img, err = resolver.BuildImage(ctx, cmdCtx.IO, opts)
		if err != nil {
			return err
		}
		if img == nil {
			return errors.New("could not find an image to deploy")
		}
	} else {
		opts := imgsrc.RefOptions{
			AppName:    cmdCtx.AppName,
			WorkingDir: cmdCtx.WorkingDir,
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageRef:   imageOrPath,
			ImageLabel: cmdCtx.Config.GetString("image-label"),
		}

		img, err = resolver.ResolveReference(ctx, cmdCtx.IO, opts)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(cmdCtx.Client.IO.Out, "Image: %s\n", img.Tag)
	fmt.Fprintf(cmdCtx.Client.IO.Out, "Image size: %s\n", humanize.Bytes(uint64(img.Size)))

	if img == nil {
		return errors.New("could not find an image to deploy")
	}

	if cmdCtx.Config.GetBool("build-only") {
		return nil
	}

	machineConf.Image = img.Tag

	if size := cmdCtx.Config.GetString("size"); size != "" {
		machineConf.VMSize = size
	}

	if entrypoint := cmdCtx.Config.GetString("entrypoint"); entrypoint != "" {
		splitted, err := shlex.Split(entrypoint)
		if err != nil {
			return errors.Wrap(err, "invalid entrypoint")
		}
		machineConf.Init.Entrypoint = splitted
	}

	if cmd := cmdCtx.Args[1:]; len(cmd) > 0 {
		machineConf.Init.Cmd = cmd
	}

	svcs := make([]interface{}, len(cmdCtx.Config.GetStringSlice("port")))

	for i, p := range cmdCtx.Config.GetStringSlice("port") {
		proto := "tcp"
		handlers := []string{}

		splittedPortsProto := strings.Split(p, "/")
		if len(splittedPortsProto) > 1 {
			splittedProtoHandlers := strings.Split(splittedPortsProto[1], ":")
			proto = splittedProtoHandlers[0]
			handlers = append(handlers, splittedProtoHandlers[1:]...)
		}

		splittedPorts := strings.Split(splittedPortsProto[0], ":")
		edgePort, err := strconv.Atoi(splittedPorts[0])
		if err != nil {
			return errors.Wrap(err, "invalid edge port")
		}
		machinePort := edgePort
		if len(splittedPorts) > 1 {
			machinePort, err = strconv.Atoi(splittedPorts[1])
			if err != nil {
				return errors.Wrap(err, "invalid machine (internal) port")
			}
		}

		svcs[i] = map[string]interface{}{
			"protocol":      proto,
			"internal_port": machinePort,
			"ports": []map[string]interface{}{
				{
					"port":     edgePort,
					"handlers": handlers,
				},
			},
		}
	}

	machineConf.Services = svcs

	var mounts []api.MachineMount

	for _, v := range cmdCtx.Config.GetStringSlice("volume") {
		splittedIDDestOpts := strings.Split(v, ":")

		mount := api.MachineMount{
			Volume: splittedIDDestOpts[0],
			Path:   splittedIDDestOpts[1],
		}

		if len(splittedIDDestOpts) > 2 {
			splittedOpts := strings.Split(splittedIDDestOpts[2], ",")
			for _, opt := range splittedOpts {
				splittedKeyValue := strings.Split(opt, "=")
				if splittedKeyValue[0] == "size" {
					i, err := strconv.Atoi(splittedKeyValue[1])
					if err != nil {
						return errors.Wrapf(err, "could not parse volume '%s' size option value '%s', must be an integer", splittedIDDestOpts[0], splittedKeyValue[1])
					}
					mount.SizeGb = i
				} else if splittedKeyValue[0] == "encrypt" {
					mount.Encrypted = true
				}
			}
		}

		mounts = append(mounts, mount)
	}

	machineConf.Mounts = mounts

	input := api.LaunchMachineInput{
		AppID:   cmdCtx.AppName,
		ID:      cmdCtx.Config.GetString("id"),
		Name:    cmdCtx.Config.GetString("name"),
		OrgSlug: org.ID,
		Region:  cmdCtx.Config.GetString("region"),
		Config:  machineConf,
	}

	machine, app, err := cmdCtx.Client.API().LaunchMachine(ctx, input)
	if err != nil {
		return err
	}

	if cmdCtx.Config.GetBool("detach") {
		fmt.Println(machine.ID)
		return nil
	}

	apiClient := cmdCtx.Client.API()

	opts := &logs.LogOptions{
		AppName: app.Name,
		VMID:    machine.ID,
	}

	stream, err := logs.NewNatsStream(ctx, apiClient, opts)

	if err != nil {
		terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
		terminal.Debug("Falling back to log polling...")

		stream, err = logs.NewPollingStream(ctx, apiClient, opts)
		if err != nil {
			return err
		}
	}

	presenter := presenters.LogPresenter{}

	entries := stream.Stream(ctx, opts)

	for {
		select {
		case <-ctx.Done():
			return stream.Err()
		case entry := <-entries:
			presenter.FPrint(cmdCtx.Out, cmdCtx.OutputJSON(), entry)
		}
	}
}
