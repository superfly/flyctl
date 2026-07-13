package machine

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
)

// guestFlagContext builds a context whose flag set contains the guest-related
// flags of `machine run`, parsed from args.
func guestFlagContext(t *testing.T, args ...string) context.Context {
	t.Helper()

	cmd := &cobra.Command{}
	flag.Add(cmd, flag.VMSizeFlags, flag.String{Name: "machine-config"})
	require.NoError(t, cmd.ParseFlags(args))

	return flag.NewContext(context.Background(), cmd.Flags())
}

func TestResolveMachineGuest(t *testing.T) {
	t.Run("defaults to shared-cpu-1x", func(t *testing.T) {
		conf := &fly.MachineConfig{}
		require.NoError(t, resolveMachineGuest(guestFlagContext(t), conf))
		assert.Equal(t, "shared", conf.Guest.CPUKind)
		assert.Equal(t, 1, conf.Guest.CPUs)
	})

	t.Run("guest from --machine-config", func(t *testing.T) {
		conf := &fly.MachineConfig{}
		ctx := guestFlagContext(t,
			"--machine-config", `{"guest":{"cpu_kind":"performance","cpus":2,"memory_mb":4096}}`,
		)
		require.NoError(t, resolveMachineGuest(ctx, conf))
		assert.Equal(t, "performance", conf.Guest.CPUKind)
		assert.Equal(t, 2, conf.Guest.CPUs)
		assert.Equal(t, 4096, conf.Guest.MemoryMB)
	})

	t.Run("--vm-* flags override --machine-config", func(t *testing.T) {
		conf := &fly.MachineConfig{}
		ctx := guestFlagContext(t,
			"--machine-config", `{"guest":{"cpu_kind":"performance","cpus":2,"memory_mb":4096}}`,
			"--vm-memory", "8192",
		)
		require.NoError(t, resolveMachineGuest(ctx, conf))
		assert.Equal(t, "performance", conf.Guest.CPUKind)
		assert.Equal(t, 8192, conf.Guest.MemoryMB)
	})
}
