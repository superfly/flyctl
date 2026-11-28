package ssh

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/flapsutil"
)

type lookupRecorder struct{ host, network string }

func (r *lookupRecorder) WaitForDNS(_ context.Context, _ agent.Dialer, _, host, network string) error {
	r.host, r.network = host, network
	return nil
}

func TestConsoleHostnameUsesAppNetwork(t *testing.T) {
	for _, tc := range []struct{ appNetwork, wantNetwork string }{
		{"blue", "blue"},
		{"default", ""},
	} {
		t.Run(tc.appNetwork, func(t *testing.T) {
			ctx, _ := sshFlagContext(t, "example.internal")
			ctx = flapsutil.NewContextWithClient(ctx, workflowSelectionClient{machines: []*fly.Machine{{ID: "m", State: "started", Config: &fly.MachineConfig{}}}})
			lookup := new(lookupRecorder)
			addr, _, err := lookupAddressAndContainer(ctx, lookup, nil, &flaps.App{Name: "test", Network: tc.appNetwork}, true)
			require.NoError(t, err)
			require.Equal(t, "example.internal", addr)
			require.Equal(t, addr, lookup.host)
			require.Equal(t, tc.wantNetwork, lookup.network)
		})
	}
}
