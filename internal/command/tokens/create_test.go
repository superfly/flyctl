package tokens

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func wireGuardCaveats(t *testing.T, args ...string) ([]macaroon.Caveat, error) {
	t.Helper()
	cmd := newWireGuard()
	require.NoError(t, cmd.ParseFlags(args))

	return getWireGuardCaveats(flag.NewContext(context.Background(), cmd.Flags()))
}

func TestWireGuardNetworkCaveats(t *testing.T) {
	const peerActions = resset.ActionWrite | resset.ActionCreate | resset.ActionDelete | resset.ActionControl

	wg := func(action resset.Action, network string) *flyio.Access {
		access := &flyio.Access{Action: action, OrgID: fly.Pointer(uint64(1)), Feature: fly.Pointer(flyio.FeatureWireGuard)}
		if network != "" {
			access.NetworkName = &network
		}

		return access
	}
	limited := []string{"--network", " one , default", "--network", "two"}

	for _, tc := range []struct {
		name    string
		args    []string
		access  *flyio.Access
		wantErr error
	}{
		{name: "write on an allowed network", args: limited, access: wg(resset.ActionWrite, "one")},
		{name: "write on a network from a repeated flag", args: limited, access: wg(resset.ActionWrite, "two")},
		{name: "peer management on an allowed network", args: limited, access: wg(peerActions, "one")},
		{name: "default is the reserved default network name", args: limited, access: wg(resset.ActionWrite, flyio.NetworkNameDefault)},
		{name: "default is not kept as a literal name", args: limited, access: wg(resset.ActionWrite, "default"), wantErr: resset.ErrUnauthorizedForResource},
		{name: "write on another network", args: limited, access: wg(resset.ActionWrite, "other"), wantErr: resset.ErrUnauthorizedForResource},
		{name: "write without a network", args: limited, access: wg(resset.ActionWrite, ""), wantErr: resset.ErrUnauthorizedForAction},
		{name: "read with only the org", args: limited, access: &flyio.Access{Action: resset.ActionRead, OrgID: fly.Pointer(uint64(1))}},
		{name: "no flag allows any network", access: wg(resset.ActionWrite, "other")},
		{name: "no flag allows a write without a network", access: wg(resset.ActionWrite, "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cavs, err := wireGuardCaveats(t, tc.args...)
			require.NoError(t, err)

			err = macaroon.NewCaveatSet(cavs...).Validate(tc.access)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}

	for _, value := range []string{"", " ", "a,,b", "a, "} {
		t.Run(fmt.Sprintf("refuses %q", value), func(t *testing.T) {
			cavs, err := wireGuardCaveats(t, "--network", value)
			require.ErrorContains(t, err, "network name")
			require.Nil(t, cavs)
		})
	}
}
