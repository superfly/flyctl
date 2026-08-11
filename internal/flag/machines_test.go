package flag

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
)

func ctxWithFlags(t *testing.T, args ...string) context.Context {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	VMSizeFlags.addTo(cmd)

	fs := cmd.Flags()
	require.NoError(t, fs.Parse(args))

	// Aliases only resolve onto their main flag once this preparer has run.
	ctx, err := preparers.ApplyAliases(NewContext(context.Background(), fs))
	require.NoError(t, err)

	return ctx
}

func TestGetMachineGuestRejectsGPUFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--vm-gpu-kind", "l40s"},
		{"--vm-gpukind", "l40s"},
		{"--vm-gpus", "1"},
	} {
		_, err := GetMachineGuest(ctxWithFlags(t, args...), nil)
		assert.ErrorContains(t, err, "GPU machines are no longer supported", "for %v", args)
	}
}

func TestGetMachineGuestWithoutGPUFlags(t *testing.T) {
	guest, err := GetMachineGuest(ctxWithFlags(t, "--vm-cpus", "2"), nil)
	require.NoError(t, err)
	assert.Equal(t, 2, guest.CPUs)
}
