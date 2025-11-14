package flapsutil

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/metrics"
)

func NewClientWithOptions(ctx context.Context, opts flaps.NewClientOpts) (*flaps.Client, error) {
	// Connect over wireguard depending on FLAPS URL.
	if strings.TrimSpace(strings.ToLower(os.Getenv("FLY_FLAPS_BASE_URL"))) == "peer" {
		if opts.OrgSlug == "" {
			orgSlug, err := resolveOrgSlugForApp(ctx, opts.AppCompact, opts.AppName)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve org for app '%s': %w", opts.AppName, err)
			}
			opts.OrgSlug = orgSlug
		}

		client := flyutil.ClientFromContext(ctx)
		agentclient, err := agent.Establish(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("error establishing agent: %w", err)
		}

		dialer, err := agentclient.Dialer(ctx, opts.OrgSlug, "")
		if err != nil {
			return nil, fmt.Errorf("flaps: can't build tunnel for %s: %w", opts.OrgSlug, err)
		}
		opts.DialContext = dialer.DialContext

		flapsBaseUrlString := fmt.Sprintf("http://[%s]:4280", resolvePeerIP(dialer.State().Peer.Peerip))
		if opts.BaseURL, err = url.Parse(flapsBaseUrlString); err != nil {
			return nil, fmt.Errorf("failed to parse flaps url '%s' with error: %w", flapsBaseUrlString, err)
		}
	}

	if opts.UserAgent == "" {
		opts.UserAgent = buildinfo.UserAgent()
	}

	if opts.Tokens == nil {
		opts.Tokens = config.Tokens(ctx)
	}

	if v := logger.MaybeFromContext(ctx); v != nil {
		opts.Logger = v
	}

	return flaps.NewWithOptions(ctx, opts)
}

func resolveOrgSlugForApp(ctx context.Context, app *fly.AppCompact, appName string) (string, error) {
	app, err := resolveApp(ctx, app, appName)
	if err != nil {
		return "", err
	}
	return app.Organization.Slug, nil
}

func resolveApp(ctx context.Context, app *fly.AppCompact, appName string) (*fly.AppCompact, error) {
	var err error
	if app == nil {
		client := flyutil.ClientFromContext(ctx)
		app, err = client.GetAppCompact(ctx, appName)
	}
	return app, err
}

func resolvePeerIP(ip string) string {
	peerIP := net.ParseIP(ip)
	var natsIPBytes [16]byte
	copy(natsIPBytes[0:], peerIP[0:6])
	natsIPBytes[15] = 3
	return net.IP(natsIPBytes[:]).String()
}

func Launch(ctx context.Context, client FlapsClient, builder fly.LaunchMachineInput) (out *fly.Machine, err error) {
	metrics.Started(ctx, "machine_launch")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_launch/duration")
	defer func() {
		metrics.Status(ctx, "machine_launch", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()
	return client.Launch(ctx, builder)
}

func Update(ctx context.Context, client FlapsClient, builder fly.LaunchMachineInput, nonce string) (out *fly.Machine, err error) {
	metrics.Started(ctx, "machine_update")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_update/duration")
	defer func() {
		metrics.Status(ctx, "machine_update", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()
	return client.Update(ctx, builder, nonce)
}

func Start(ctx context.Context, client FlapsClient, machineID string, nonce string) (out *fly.MachineStartResponse, err error) {
	metrics.Started(ctx, "machine_start")
	defer func() {
		metrics.Status(ctx, "machine_start", err == nil)
	}()
	return client.Start(ctx, machineID, nonce)
}

func Cordon(ctx context.Context, client FlapsClient, machineID string, nonce string) (err error) {
	metrics.Started(ctx, "machine_cordon")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_cordon/duration")
	defer func() {
		metrics.Status(ctx, "machine_cordon", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()
	return client.Cordon(ctx, machineID, nonce)
}

func Uncordon(ctx context.Context, client FlapsClient, machineID string, nonce string) (err error) {
	metrics.Started(ctx, "machine_uncordon")
	sendUpdateMetrics := metrics.StartTiming(ctx, "machine_uncordon/duration")
	defer func() {
		metrics.Status(ctx, "machine_uncordon", err == nil)
		if err == nil {
			sendUpdateMetrics()
		}
	}()
	return client.Uncordon(ctx, machineID, nonce)
}
