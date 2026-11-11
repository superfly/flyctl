package wireguard

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/wg"
)

func TestStateForOrgChecksSavedPeerOrganization(t *testing.T) {
	for _, savedOrg := range []string{"personal", "", "personal-b"} {
		t.Run("saved_org="+savedOrg, func(t *testing.T) {
			t.Chdir(t.TempDir())
			viper.Reset()
			t.Cleanup(viper.Reset)
			ctx := state.WithConfigDirectory(context.Background(), t.TempDir())
			// The legacy key for personal on network b is also the key for
			// canonical org personal-b on the default network.
			saved := &wg.WireGuardState{Org: savedOrg, Name: "saved-peer", Region: "iad"}
			viper.Set(flyctl.ConfigWireGuardState, wg.States{"personal-b": saved})
			creates := 0
			client := &mock.Client{CreateWireGuardPeerFunc: func(_ context.Context, orgID, region, name, pubkey, network string) (*fly.CreatedWireGuardPeer, error) {
				creates++
				require.Equal(t, "org-b", orgID)
				require.Empty(t, network)
				return &fly.CreatedWireGuardPeer{Peerip: "fdaa:0:2::100"}, nil
			}}
			got, err := StateForOrg(ctx, client, "org-b", "personal-b", "iad", "fresh-peer", false, "")
			require.NoError(t, err)
			require.Equal(t, "personal-b", got.Org)
			if savedOrg == "personal-b" {
				require.Equal(t, saved, got)
				require.Zero(t, creates)
				return
			}
			require.Equal(t, 1, creates)
			require.Equal(t, "fresh-peer", got.Name)
			// A restarted agent must reuse the replacement, not register again.
			viper.Reset()
			viper.SetConfigFile(state.ConfigFile(ctx))
			require.NoError(t, viper.ReadInConfig())
			reloaded, err := StateForOrg(ctx, client, "org-b", "personal-b", "iad", "unused", false, "")
			require.NoError(t, err)
			require.Equal(t, got, reloaded)
			require.Equal(t, 1, creates)
		})
	}
}
