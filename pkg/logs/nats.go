package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"net"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/terminal"
	"gopkg.in/yaml.v2"
)

type natsLogStream struct {
	nc  *nats.Conn
	err error
}

func NewNatsStream(apiClient *api.Client, opts *LogOptions) (LogStream, error) {

	ctx := context.Background()

	app, err := apiClient.GetApp(opts.AppName)
	if err != nil {
		return nil, errors.Wrap(err, "error fetching target app")
	}

	agentclient, err := agent.Establish(ctx, apiClient)
	if err != nil {
		return nil, errors.Wrap(err, "error establishing agent")
	}

	dialer, err := agentclient.Dialer(ctx, &app.Organization)
	if err != nil {
		return nil, errors.Wrapf(err, "error establishing wireguard connection for %s organization", app.Organization.Slug)
	}

	tunnelCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	// wait for the tunnel to be ready
	if err = agentclient.WaitForTunnel(tunnelCtx, &app.Organization); err != nil {
		return nil, errors.Wrap(err, "unable to connect WireGuard tunnel")
	}

	nc, err := newNatsClient(ctx, dialer, app)
	if err != nil {
		return nil, errors.Wrap(err, "error creating nats connection")
	}

	return &natsLogStream{nc: nc}, nil
}

// natsLogStream implements LogStream
func (n *natsLogStream) Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry {
	out := make(chan LogEntry)

	subject := fmt.Sprintf("logs.%s", opts.AppName)

	if opts.RegionCode != "" {
		subject = fmt.Sprintf("%s.%s", subject, opts.RegionCode)
	} else {
		subject = fmt.Sprintf("%s.%s", subject, "*")
	}
	if opts.VMID != "" {
		subject = fmt.Sprintf("%s.%s", subject, opts.VMID)
	}
	subject = fmt.Sprintf("%s.%s", subject, ">")

	terminal.Debug("subscribing to nats subject: ", subject)

	sub, err := n.nc.Subscribe(subject, func(msg *nats.Msg) {
		go func() {
			var log natsLog
			if err := json.Unmarshal(msg.Data, &log); err != nil {
				terminal.Error(errors.Wrap(err, "could not parse log"))
				return
			}

			out <- LogEntry{
				Instance:  log.Fly.App.Instance,
				Level:     log.Log.Level,
				Message:   log.Message,
				Timestamp: log.Timestamp,
				Meta: Meta{
					Instance: log.Fly.App.Instance,
					Region:   log.Fly.Region,
					Event:    struct{ Provider string }{log.Event.Provider},
				},
			}

			<-ctx.Done()

		}()

	})
	if err != nil {
		n.err = errors.Wrap(err, "could not sub to logs via nats")
		return nil
	}
	go func() {
		<-ctx.Done()
		sub.Unsubscribe()
	}()

	return out
}

func newNatsClient(ctx context.Context, dialer agent.Dialer, app *api.App) (*nats.Conn, error) {

	var flyConf flyConfig
	usr, _ := user.Current()
	flyConfFile, err := os.Open(filepath.Join(usr.HomeDir, ".fly", "config.yml"))
	if err != nil {
		return nil, errors.Wrap(err, "could not read fly config yml")
	}
	if err := yaml.NewDecoder(flyConfFile).Decode(&flyConf); err != nil {
		return nil, errors.Wrap(err, "could not decode fly config yml")
	}

	state, ok := flyConf.WireGuardState[app.Organization.Slug]
	if !ok {
		return nil, errors.New("could not find org in fly config")
	}

	peerIP := state.Peer.PeerIP

	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3

	natsIP := net.IP(natsIPBytes[:])

	terminal.Debug("connecting to nats")

	conn, err := nats.Connect(fmt.Sprintf("nats://[%s]:4223", natsIP.String()), nats.SetCustomDialer(&natsDialer{dialer, ctx}), nats.UserInfo(app.Organization.Slug, flyConf.AccessToken))
	if err != nil {
		return nil, errors.Wrap(err, "could not connect to nats")
	}
	return conn, nil
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
