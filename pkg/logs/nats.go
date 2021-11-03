package logs

import (
	"context"
	"encoding/json"
	"fmt"

	"net"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/terminal"
)

type natsLogStream struct {
	nc  *nats.Conn
	err error
}

func NewNatsStream(ctx context.Context, apiClient *api.Client, opts *LogOptions) (LogStream, error) {
	app, err := apiClient.GetApp(ctx, opts.AppName)
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
func (s *natsLogStream) Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry {
	out := make(chan LogEntry)

	subject := fmt.Sprintf("logs.%s", opts.AppName)

	if opts.RegionCode != "" {
		subject = fmt.Sprintf("%s.%s", subject, opts.RegionCode)
	} else {
		subject = fmt.Sprintf("%s.%s", subject, "*")
	}
	if opts.VMID != "" {
		subject = fmt.Sprintf("%s.%s", subject, opts.VMID)
	} else {
		subject = fmt.Sprintf("%s.%s", subject, "*")
	}

	terminal.Debug("subscribing to nats subject: ", subject)

	sub, err := s.nc.Subscribe(subject, func(msg *nats.Msg) {

		var log natsLog

		if err := json.Unmarshal(msg.Data, &log); err != nil {
			terminal.Error(errors.Wrap(err, "could not parse log"))
			return
		}

		out <- LogEntry{
			Instance:  log.Fly.App.Instance,
			Level:     log.Log.Level,
			Message:   log.Message,
			Region:    log.Fly.Region,
			Timestamp: log.Timestamp,
			Meta: Meta{
				Instance: log.Fly.App.Instance,
				Region:   log.Fly.Region,
				Event:    struct{ Provider string }{log.Event.Provider},
			},
		}

	})
	if err != nil {
		s.err = errors.Wrap(err, "could not sub to logs via nats")
		return nil
	}
	go func() {
		defer sub.Unsubscribe()
		defer close(out)

		<-ctx.Done()

	}()

	return out
}

func (s *natsLogStream) Err() error {
	return s.err
}

func newNatsClient(ctx context.Context, dialer agent.Dialer, app *api.App) (*nats.Conn, error) {

	state := dialer.State()

	peerIP := net.ParseIP(state.Peer.Peerip)

	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3

	natsIP := net.IP(natsIPBytes[:])

	terminal.Debug("connecting to nats server: ", natsIP.String())

	conn, err := nats.Connect(fmt.Sprintf("nats://[%s]:4223", natsIP.String()), nats.SetCustomDialer(&natsDialer{dialer, ctx}), nats.UserInfo(app.Organization.Slug, flyctl.GetAPIToken()))
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
