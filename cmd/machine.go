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

	"github.com/google/shlex"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v2"
)

func newMachineCommand(client *client.Client) *Command {
	cmd := BuildCommandKS(nil, nil, docstrings.Get("machine"), client)

	newMachineRunCommand(cmd, client)
	newMachineStopCommand(cmd, client)
	newMachineListCommand(cmd, client)

	return cmd
}

func newMachineListCommand(parent *Command, client *client.Client) {
	BuildCommandKS(parent, runMachineList, docstrings.Get("machine.list"), client, requireSession)
}

func newMachineStopCommand(parent *Command, client *client.Client) {
	cmd := BuildCommandKS(parent, runMachineStop, docstrings.Get("machine.stop"), client, requireSession)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "signal",
		Shorthand:   "s",
		Description: "Signal to stop the machine with (default: SIGINT)",
	})

	cmd.AddIntFlag(IntFlagOpts{
		Name:        "time",
		Description: "Seconds to wait before killing the machine",
	})

	cmd.Args = cobra.ExactArgs(1)
}

func newMachineRunCommand(parent *Command, client *client.Client) {
	cmd := BuildCommandKS(parent, runMachineRun, docstrings.Get("machine.run"), client, requireSession)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Shorthand:   "n",
		Description: "App name, will be generated if missing",
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

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "entrypoint",
		Description: "ENTRYPOINT replacement",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Shorthand:   "d",
		Description: "Print machine id and exit",
	})

	cmd.Command.Args = cobra.MinimumNArgs(1)
}

func runMachineList(cmdCtx *cmdctx.CmdContext) error {
	machines, err := cmdCtx.Client.API().ListMachines("running")
	if err != nil {
		return errors.Wrap(err, "could not get list of machines")
	}

	for _, machine := range machines {
		fmt.Printf("%s : %s\n", machine.App.Name, machine.ID)
	}

	return nil
}

func runMachineStop(cmdCtx *cmdctx.CmdContext) error {
	input := api.StopMachineInput{
		ID:              cmdCtx.Args[0],
		Signal:          cmdCtx.Config.GetString("signal"),
		KillTimeoutSecs: cmdCtx.Config.GetInt("time"),
	}

	machine, err := cmdCtx.Client.API().StopMachine(input)
	if err != nil {
		return errors.Wrap(err, "could not stop machine")
	}

	fmt.Println(machine.ID)

	return nil
}

func runMachineRun(cmdCtx *cmdctx.CmdContext) error {
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

	appName := cmdCtx.Config.GetString("name")
	if appName == "" {
		appName = cmdCtx.AppName
	}

	apiMachineConf := api.MachineConfig(machineConf.Config)

	input := api.LaunchMachineInput{
		AppName: appName,
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

	ctx := createCancellableContext()
	eg, errCtx := errgroup.WithContext(ctx)

	dialerCh := make(chan *agent.Dialer, 1)

	eg.Go(func() error {
		agentclient, err := agent.Establish(errCtx, apiClient)
		if err != nil {
			return errors.Wrap(err, "error establishing agent")
		}

		dialer, err := agentclient.Dialer(errCtx, &app.Organization)
		if err != nil {
			return errors.Wrapf(err, "error establishing wireguard connection for %s organization", app.Organization.Slug)
		}

		tunnelCtx, cancel := context.WithTimeout(errCtx, 4*time.Minute)
		defer cancel()
		if err = agentclient.WaitForTunnel(tunnelCtx, &app.Organization); err != nil {
			return errors.Wrap(err, "unable to connect WireGuard tunnel")
		}

		dialerCh <- dialer

		return nil
	})

	if err = eg.Wait(); err != nil {
		return err
	}

	dialer := <-dialerCh

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
		fmt.Println(log.Message)
	})
	if err != nil {
		return errors.Wrap(err, "could not sub to logs via nats")
	}
	defer sub.Unsubscribe()

	<-ctx.Done()

	return nil
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
	*agent.Dialer
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
