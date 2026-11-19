package wireguard

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/flyctl/wg"
)

func runWireguardList(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	peers, err := apiClient.GetWireGuardPeers(ctx, org.Slug)
	if err != nil {
		return err
	}

	// the agent keys tunnels by the real slug; uiex calls the personal
	// org "personal"
	agentSlug := org.RawSlug
	if agentSlug == "" {
		agentSlug = org.Slug
	}

	tokenTunnels := tokenTunnelsFor(ctx, agentSlug)

	if config.FromContext(ctx).JSONOutput {
		if len(tokenTunnels) == 0 {
			render.JSON(io.Out, peers)

			return nil
		}

		rows := make([]any, 0, len(peers)+len(tokenTunnels))
		for _, peer := range peers {
			rows = append(rows, peer)
		}

		for _, t := range tokenTunnels {
			rows = append(rows, tokenTunnelRow{Pubkey: t.Pubkey, Name: t.Name, Peerip: t.PeerIP, Endpoint: t.Endpoint})
		}

		render.JSON(io.Out, rows)

		return nil
	}

	table := tablewriter.NewTable(io.Out,
		tablewriter.WithHeader([]string{"Name", "Region", "Peer IP"}),
	)

	for _, peer := range peers {
		table.Append(peer.Name, peer.Region, peer.Peerip) //nolint:errcheck
	}

	table.Render() //nolint:errcheck

	if len(tokenTunnels) == 0 {
		return nil
	}

	fmt.Fprintln(io.Out)
	fmt.Fprintln(io.Out, "Token-mode tunnels in use by this machine's agent:")

	table = tablewriter.NewTable(io.Out,
		tablewriter.WithHeader([]string{"Name", "Peer IP", "Endpoint"}),
	)

	for _, t := range tokenTunnels {
		table.Append(t.Name, t.PeerIP, t.Endpoint) //nolint:errcheck
	}

	table.Render() //nolint:errcheck

	return nil
}

// tokenTunnelRow is a token-mode tunnel in list's JSON output, shaped like
// the API's peers (with no ID or region: it isn't registered) plus where its
// WireGuard goes.
type tokenTunnelRow struct {
	ID       string
	Pubkey   string
	Region   string
	Name     string
	Peerip   string
	Endpoint string
}

// agentTunnelTimeout bounds asking the agent, which may be busy building a
// tunnel: the API's peers shouldn't wait long on it.
const agentTunnelTimeout = 5 * time.Second

// agentTunnels asks a running agent for its tunnels for the org. A variable
// so tests can stand in for the agent.
var agentTunnels = func(ctx context.Context, slug string) ([]wg.TunnelStatus, error) {
	client, err := agent.DefaultClient(ctx)
	if err != nil {
		return nil, err
	}

	return client.Tunnels(ctx, slug)
}

// tokenTunnelsFor returns the token-mode tunnels this machine's agent holds
// for the org. Their peers live only at the gateway, so the API doesn't list
// them. Without a running agent there are none; it isn't started for this.
// Trouble asking the agent is a warning: the API's peers still get listed.
func tokenTunnelsFor(ctx context.Context, slug string) []wg.TunnelStatus {
	ctx, cancel := context.WithTimeout(ctx, agentTunnelTimeout)
	defer cancel()

	tunnels, err := agentTunnels(ctx, slug)

	switch {
	case errors.Is(err, agent.ErrAgentNotRunning):
		return nil
	case errors.Is(err, agent.ErrUnsupportedCommand):
		fmt.Fprintln(iostreams.FromContext(ctx).ErrOut, "The running agent is too old to report token-mode tunnels; run 'fly agent restart' to list them.")

		return nil
	case err != nil:
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut, "Couldn't get token-mode tunnels from the agent: %v\n", err)

		return nil
	}

	var token []wg.TunnelStatus
	for _, t := range tunnels {
		if t.Token {
			token = append(token, t)
		}
	}

	return token
}

func runWireguardWebsockets(ctx context.Context) error {
	return setAgentToggle(ctx, "websockets", flyctl.ConfigWireGuardWebsockets, config.SetWireGuardWebsocketsEnabled)
}

func runWireguardTokenMode(ctx context.Context) error {
	return setAgentToggle(ctx, "token-mode", flyctl.ConfigWireGuardTokenMode, config.SetWireGuardTokenModeEnabled)
}

// setAgentToggle handles an (enable|disable) subcommand that changes how the
// agent builds tunnels: it persists the setting and restarts the agent so
// the change takes effect.
func setAgentToggle(ctx context.Context, name, key string, save func(path string, enabled bool) error) error {
	var enabled bool
	switch arg := flag.FirstArg(ctx); arg {
	case "enable":
		enabled = true
	case "disable":
		enabled = false
	default:
		return fmt.Errorf("bad arg %q: flyctl wireguard %s (enable|disable)", arg, name)
	}

	viper.Set(key, enabled)
	if err := save(state.ConfigFile(ctx), enabled); err != nil {
		return errors.Wrap(err, "error saving config file")
	}

	restartAgentForConfigChange(ctx)

	return nil
}

// restartAgentForConfigChange stops the running agent so the next command
// starts one that picks up the new transport setting; if that fails it
// prints manual instructions.
func restartAgentForConfigChange(ctx context.Context) {
	io := iostreams.FromContext(ctx)

	tryKillingAgent := func() error {
		client, err := agent.DefaultClient(ctx)
		if err == agent.ErrAgentNotRunning {
			return nil
		} else if err != nil {
			return err
		}

		return client.Kill(ctx)
	}

	if err := tryKillingAgent(); err != nil {
		terminal.Debugf("error stopping the agent: %s", err)
		fmt.Fprintf(io.Out, "Run `flyctl agent restart` to make changes take effect.\n")
	}
}

func runWireguardReset(ctx context.Context) error {
	io := iostreams.FromContext(ctx)

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	apiClient := flyutil.ClientFromContext(ctx)
	agentclient, err := agent.Establish(ctx, apiClient)
	if err != nil {
		return err
	}

	conf, err := agentclient.Reestablish(ctx, org.Slug, "")
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "New WireGuard peer for organization '%s': '%s'\n", org.Slug, conf.WireGuardState.Name)

	return nil
}

func runWireguardCreate(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	args := flag.Args(ctx)
	var region string
	var name string

	if len(args) > 1 && args[1] != "" {
		region = args[1]
	}

	if len(args) > 2 && args[2] != "" {
		name = args[2]
	}

	network := flag.GetString(ctx, "network")

	fmt.Fprintf(io.Out, `
!!!! WARNING: Output includes private key. Private keys cannot be recovered !!!!
!!!! after creating the peer; if you lose the key, you'll need to remove    !!!!
!!!! and re-add the peering connection.                                     !!!!
`)

	// Settle where the configuration goes before the peer exists: its private
	// key is never stored anywhere else, so it must have a destination first.
	w, shouldClose, err := resolveOutputWriter(ctx, 3, "Filename to store WireGuard configuration in, or 'stdout': ")
	if err != nil {
		return err
	}
	if shouldClose {
		defer w.Close() // skipcq: GO-S2307
	}

	state, err := wireguard.Create(apiClient, org.ID, org.Slug, region, name, network, "static")
	if err != nil {
		if shouldClose {
			// The file was created empty for this peer; remove it so a retry
			// can use the same name.
			filename := w.(*os.File).Name()
			_ = w.Close()
			_ = os.Remove(filename)
		}

		return err
	}

	data := &state.Peer

	generateWgConf(data, state.LocalPrivate, w)

	if shouldClose {
		filename := w.(*os.File).Name()
		fmt.Fprintf(io.Out, "Wrote WireGuard configuration to %s; load in your WireGuard client\n", filename)
	}

	return nil
}

func runWireguardRemove(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	args := flag.Args(ctx)
	var name string
	if len(args) >= 2 {
		name = args[1]
	} else {
		name, err = selectWireGuardPeer(ctx, apiClient, org.Slug)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(io.Out, "Removing WireGuard peer \"%s\" for organization %s\n", name, org.Slug)

	err = apiClient.RemoveWireGuardPeer(ctx, org.ID, name)
	if err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Removed peer.")

	return wireguard.PruneInvalidPeers(ctx, apiClient)
}
