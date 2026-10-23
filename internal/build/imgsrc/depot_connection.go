package imgsrc

import (
	"context"
	"fmt"
	"strings"
	"time"

	depotmachine "github.com/depot/depot-go/machine"
	"github.com/moby/buildkit/client"
	"github.com/superfly/flyctl/internal/uiex"
)

// Keep SDK client/TLS/Release ownership and the one-second readiness retry.
// Unlike Connect, retain the last completed error when cancellation wins.
func connectDepotSDK(ctx context.Context, machine *depotmachine.Machine, meta *uiex.BuilderMeta) (c *client.Client, err error) {
	var last error
	defer func() {
		meta.DepotConnectionStage = depotConnectionStage(last)
		if err == nil {
			meta.DepotConnectionStage = "ready"
		}
		meta.DepotConnectionOutcome = depotConnectOutcome(ctx, err)
	}()
	for {
		if ctx.Err() != nil {
			if last == nil {
				return nil, ctx.Err()
			}
			if ctx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("timed out connecting to machine: %w: %w", ctx.Err(), last)
			}

			return nil, fmt.Errorf("%w: %w", ctx.Err(), last)
		}
		c, err = machine.CheckReady(ctx)
		if err == nil {
			return c, nil
		}
		// A terminal context-only error must not erase an earlier observation.
		if ctx.Err() == nil || last == nil || depotConnectionStage(err) != "connecting" {
			last = err
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

// These markers belong to the pinned BuildKit/gRPC transport. They describe
// observed failures, not proof of an ingress authentication or routing cause.
// Unknown errors (including a still-pending handshake) stay connecting.
func depotConnectionStage(err error) string {
	if err == nil {
		return "connecting"
	}
	switch text := err.Error(); {
	case strings.Contains(text, "transport: Error while dialing:"):
		return "tcp_connect"
	case strings.Contains(text, "transport: authentication handshake failed:"):
		return "tls_handshake"
	case strings.Contains(text, "error reading server preface:"):
		return "post_handshake"
	case strings.Contains(text, "could not read certificate/key:"), strings.Contains(text, "failed to append ca certs"):
		return "client_configuration"
	default:
		return "connecting"
	}
}

func depotConnectOutcome(ctx context.Context, err error) string {
	if err == nil {
		return "success"
	}
	switch ctx.Err() {
	case context.Canceled:
		return "canceled"
	case context.DeadlineExceeded:
		return "deadline_exceeded"
	default:
		return "error"
	}
}
