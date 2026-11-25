package machine

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
)

func TestStandbyForFlag(t *testing.T) {
	standby := func() *fly.MachineConfig {
		return &fly.MachineConfig{
			Standbys: []string{"148e123456789a"},
			Env:      map[string]string{"FLY_STANDBY_FOR": "148e123456789a", "KEEP": "me"},
		}
	}

	for _, tc := range []struct {
		name         string
		initial      *fly.MachineConfig
		args         []string
		wantStandbys []string
	}{
		{name: "a value doesn't create a standby", initial: &fly.MachineConfig{Env: map[string]string{}}, args: []string{"--standby-for=148e123456789a"}},
		{name: "a value doesn't change a standby", initial: standby(), args: []string{"--standby-for=e2865123456789"}, wantStandbys: []string{"148e123456789a"}},
		{name: "unset keeps a standby", initial: standby(), wantStandbys: []string{"148e123456789a"}},
		{name: "empty clears a standby", initial: standby(), args: []string{"--standby-for="}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newUpdate()
			require.NoError(t, cmd.Flags().Parse(tc.args))
			applyStandbyForFlag(flag.NewContext(t.Context(), cmd.Flags()), tc.initial)
			require.Equal(t, tc.wantStandbys, tc.initial.Standbys)
			if len(tc.wantStandbys) > 0 {
				require.Equal(t, "148e123456789a", tc.initial.Env["FLY_STANDBY_FOR"])
			} else {
				require.NotContains(t, tc.initial.Env, "FLY_STANDBY_FOR")
			}
		})
	}

	for _, cmd := range []*cobra.Command{newRun(), newCreate(), newUpdate(), newClone()} {
		f := cmd.Flags().Lookup("standby-for")
		require.NotNil(t, f)
		require.True(t, f.Hidden)
		require.NotEmpty(t, f.Deprecated)
	}
}
