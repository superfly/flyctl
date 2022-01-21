package logs

import (
	"context"
	"encoding/json"
	"fmt"

	"net"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/pkg/agent"
)

type natsLogStream struct {
	nc  *nats.Conn
	err error
}

func NewNatsStream(ctx context.Context, apiClient *api.Client, opts *LogOptions) (LogStream, error) {
	app, err := apiClient.GetApp(ctx, opts.AppName)
	if err != nil {
		return nil, fmt.Errorf("failed fetching target app: %w", err)
	}

	agentclient, err := agent.Establish(ctx, apiClient)
	if err != nil {
		return nil, fmt.Errorf("failed establishing agent: %w", err)
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed establishing wireguard connection for %s organization: %w", app.Organization.Slug, err)
	}

	tunnelCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()

	if err = agentclient.WaitForTunnel(tunnelCtx, &app.Organization); err != nil {
		return nil, fmt.Errorf("failed connecting to WireGuard tunnel: %w", err)
	}

	nc, err := newNatsClient(ctx, dialer, app.Organization.Slug)
	if err != nil {
		return nil, fmt.Errorf("failed creating nats connection: %w", err)
	}

	return &natsLogStream{nc: nc}, nil
}

// natsLogStream implements LogStream
func (s *natsLogStream) Stream(ctx context.Context, opts *LogOptions) <-chan LogEntry {
	out := make(chan LogEntry)

	go func() {
		defer close(out)

		s.err = fromNats(ctx, out, s.nc, opts)
	}()

	return out
}

func (s *natsLogStream) Err() error {
	return s.err
}

func newNatsClient(ctx context.Context, dialer agent.Dialer, orgSlug string) (*nats.Conn, error) {
	state := dialer.State()

	peerIP := net.ParseIP(state.Peer.Peerip)

	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3

	natsIP := net.IP(natsIPBytes[:])

	url := fmt.Sprintf("nats://[%s]:4223", natsIP.String())
	conn, err := nats.Connect(url, nats.SetCustomDialer(&natsDialer{dialer, ctx}), nats.UserInfo(orgSlug, flyctl.GetAPIToken()))
	if err != nil {
		return nil, fmt.Errorf("failed connecting to nats: %w", err)
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

func fromNats(ctx context.Context, out chan<- LogEntry, nc *nats.Conn, opts *LogOptions) (err error) {
	var sub *nats.Subscription
	if sub, err = nc.SubscribeSync(opts.toNatsSubject()); err != nil {
		return
	}
	defer sub.Unsubscribe()

	var log natsLog
	for {
		var msg *nats.Msg
		if msg, err = sub.NextMsgWithContext(ctx); err != nil {
			break
		}

		if err = json.Unmarshal(msg.Data, &log); err != nil {
			err = fmt.Errorf("failed parsing log: %w", err)

			break
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
	}

	return
}
