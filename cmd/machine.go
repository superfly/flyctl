package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	surveyterminal "github.com/AlecAivazis/survey/v2/terminal"
	"github.com/google/shlex"
	"github.com/logrusorgru/aurora"
	"github.com/nats-io/nats.go"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/monitor"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/terminal"
	"gopkg.in/yaml.v2"
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
	state := cmdCtx.Config.GetString("state")
	if cmdCtx.Config.GetBool("all") {
		state = ""
	}
	machines, err := cmdCtx.Client.API().ListMachines(cmdCtx.AppName, state)
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
		row := []string{
			machine.ID,
			machine.Config["image"].(string),
			machine.CreatedAt.String(),
			machine.State,
			machine.Region,
			machine.Name,
		}
		if cmdCtx.AppName == "" {
			row = append(row, machine.App.Name)
		}
		data = append(data, row)
	}

	table := tablewriter.NewWriter(os.Stdout)
	headers := []string{"ID", "Image", "Created", "State", "Region", "Name"}
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
	for _, arg := range cmdCtx.Args {
		input := api.StopMachineInput{
			AppID:           cmdCtx.AppName,
			ID:              arg,
			Signal:          cmdCtx.Config.GetString("signal"),
			KillTimeoutSecs: cmdCtx.Config.GetInt("time"),
		}

		machine, err := cmdCtx.Client.API().StopMachine(input)
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
	input := api.StartMachineInput{
		AppID: cmdCtx.AppName,
		ID:    cmdCtx.Args[0],
	}

	machine, err := cmdCtx.Client.API().StartMachine(input)
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
	for _, arg := range cmdCtx.Args {
		input := api.KillMachineInput{
			AppID: cmdCtx.AppName,
			ID:    arg,
		}

		machine, err := cmdCtx.Client.API().KillMachine(input)
		if err != nil {
			return errors.Wrap(err, "could not stop machine")
		}

		fmt.Println(machine.ID)
	}

	return nil
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
	for _, arg := range cmdCtx.Args {
		input := api.RemoveMachineInput{
			AppID: cmdCtx.AppName,
			ID:    arg,
			Kill:  cmdCtx.Config.GetBool("force"),
		}

		machine, err := cmdCtx.Client.API().RemoveMachine(input)
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

	cmd.Command.Args = cobra.MinimumNArgs(1)
}

func runMachineRun(cmdCtx *cmdctx.CmdContext) error {
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

	if cmdCtx.MachineConfig == nil {
		cmdCtx.MachineConfig = flyctl.NewMachineConfig()
	}

	if extraEnv := cmdCtx.Config.GetStringSlice("env"); len(extraEnv) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("env"))
		if err != nil {
			return errors.Wrap(err, "invalid env")
		}
		cmdCtx.MachineConfig.SetEnvVariables(parsedEnv)
	}

	machineConf := cmdCtx.MachineConfig

	machineConf.Config["image"] = cmdCtx.Args[0]

	if size := cmdCtx.Config.GetString("size"); size != "" {
		machineConf.Config["size"] = size
	}

	init := map[string]interface{}{}

	if entrypoint := cmdCtx.Config.GetString("entrypoint"); entrypoint != "" {
		splitted, err := shlex.Split(entrypoint)
		if err != nil {
			return errors.Wrap(err, "invalid entrypoint")
		}
		init["entrypoint"] = splitted
	}

	if cmd := cmdCtx.Args[1:]; len(cmd) > 0 {
		init["cmd"] = cmd
	}

	machineConf.Config["init"] = init

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

	machineConf.Config["services"] = svcs

	mounts := make([]interface{}, len(cmdCtx.Config.GetStringSlice("volume")))

	for i, v := range cmdCtx.Config.GetStringSlice("volume") {
		splittedIDDestOpts := strings.Split(v, ":")

		vol := map[string]interface{}{
			"volume": splittedIDDestOpts[0],
			"path":   splittedIDDestOpts[1],
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
					vol["size_gb"] = i
				} else if splittedKeyValue[0] == "encrypt" {
					vol["encrypted"] = true
				}
			}
		}

		mounts[i] = vol
	}

	machineConf.Config["mounts"] = mounts

	apiMachineConf := api.MachineConfig(machineConf.Config)

	input := api.LaunchMachineInput{
		AppID:   cmdCtx.AppName,
		ID:      cmdCtx.Config.GetString("id"),
		Name:    cmdCtx.Config.GetString("name"),
		OrgSlug: cmdCtx.Config.GetString("org"),
		Region:  cmdCtx.Config.GetString("region"),
		Config:  &apiMachineConf,
	}

	machine, app, err := cmdCtx.Client.API().LaunchMachine(input)
	if err != nil {
		return err
	}

	if cmdCtx.Config.GetBool("detach") {
		fmt.Println(machine.ID)
		return nil
	}

	apiClient := cmdCtx.Client.API()

	dialer, err := func() (agent.Dialer, error) {
		ctx := createCancellableContext()
		agentclient, err := agent.Establish(ctx, apiClient)
		if err != nil {
			return nil, errors.Wrap(err, "error establishing agent")
		}

		dialer, err := agentclient.Dialer(ctx, &app.Organization)
		if err != nil {
			return nil, errors.Wrapf(err, "error establishing wireguard connection for %s organization", app.Organization.Slug)
		}

		tunnelCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		if err = agentclient.WaitForTunnel(tunnelCtx, &app.Organization); err != nil {
			return nil, errors.Wrap(err, "unable to connect WireGuard tunnel")
		}

		return dialer, nil
	}()
	if err != nil {
		terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
		terminal.Debug("Falling back to log polling...")
		err := monitor.WatchLogs(cmdCtx, cmdCtx.Out, monitor.LogOptions{
			AppName: app.Name,
			VMID:    machine.ID,
		})

		return err
	}

	var flyConf flyConfig
	usr, _ := user.Current()
	flyConfFile, err := os.Open(filepath.Join(usr.HomeDir, ".fly", "config.yml"))
	if err != nil {
		return errors.Wrap(err, "could not read fly config yml")
	}
	if err := yaml.NewDecoder(flyConfFile).Decode(&flyConf); err != nil {
		return errors.Wrap(err, "could not decode fly config yml")
	}

	state, ok := flyConf.WireGuardState[app.Organization.Slug]
	if !ok {
		return errors.New("could not find org in fly config")
	}

	peerIP := state.Peer.PeerIP

	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3

	natsIP := net.IP(natsIPBytes[:])

	ctx := createCancellableContext()
	natsConn, err := nats.Connect(fmt.Sprintf("nats://[%s]:4223", natsIP.String()), nats.SetCustomDialer(&natsDialer{dialer, ctx}), nats.UserInfo(app.Organization.Slug, flyConf.AccessToken))
	if err != nil {
		return errors.Wrap(err, "could not connect to nats")
	}

	sub, err := natsConn.Subscribe(fmt.Sprintf("logs.%s.*.%s", app.Name, machine.ID), func(msg *nats.Msg) {
		var log natsLog
		if err := json.Unmarshal(msg.Data, &log); err != nil {
			terminal.Error(errors.Wrap(err, "could not parse log"))
			return
		}
		w := os.Stdout
		fmt.Fprintf(w, "%s ", aurora.Faint(log.Timestamp))
		fmt.Fprintf(w, "%s[%s]", log.Event.Provider, log.Fly.App.Instance)
		fmt.Fprint(w, " ")
		fmt.Fprintf(w, "%s ", aurora.Green(log.Fly.Region))
		fmt.Fprintf(w, "[%s] ", aurora.Colorize(log.Log.Level, levelColor(log.Log.Level)))
		_, _ = w.Write([]byte(log.Message))
		fmt.Fprintln(w, "")
	})
	if err != nil {
		return errors.Wrap(err, "could not sub to logs via nats")
	}
	defer sub.Unsubscribe()

	<-ctx.Done()

	return nil
}

func levelColor(level string) aurora.Color {
	switch level {
	case "debug":
		return aurora.CyanFg
	case "info":
		return aurora.BlueFg
	case "warn":
	case "warning":
		return aurora.YellowFg
	}
	return aurora.RedFg
}

type natsLog struct {
	Event struct {
		Provider string `json:"provider"`
	} `json:"event"`
	Fly struct {
		App struct {
			Instance string `json:"instance"`
			Name     string `json:"name"`
		} `json:"app"`
		Region string `json:"region"`
	} `json:"fly"`
	Host string `json:"host"`
	Log  struct {
		Level string `json:"level"`
	} `json:"log"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type natsDialer struct {
	agent.Dialer
	ctx context.Context
}

func (d *natsDialer) Dial(network, address string) (net.Conn, error) {
	return d.Dialer.DialContext(d.ctx, network, address)
}

type flyConfig struct {
	AccessToken    string             `yaml:"access_token"`
	WireGuardState map[string]wgState `yaml:"wire_guard_state"`
}

type wgState struct {
	Peer wgPeer `yaml:"peer"`
}

type wgPeer struct {
	PeerIP net.IP `yaml:"peerip"`
}
