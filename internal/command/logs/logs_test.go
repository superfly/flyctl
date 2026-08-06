package logs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/cmdutil/preparers"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func TestMachineSelectionFlags(t *testing.T) {
	cmd := New()

	machine := cmd.Flags().Lookup("machine")
	require.NotNil(t, machine)
	assert.Equal(t, "m", machine.Shorthand)

	instance := cmd.Flags().Lookup("instance")
	require.NotNil(t, instance)
	assert.Equal(t, "i", instance.Shorthand)
	assert.True(t, instance.Hidden)

	selectFlag := cmd.Flags().Lookup("select")
	require.NotNil(t, selectFlag)
	assert.Equal(t, "s", selectFlag.Shorthand)
}

func TestResolveMachineID(t *testing.T) {
	t.Run("uses the explicit machine without listing", func(t *testing.T) {
		ctx := machineSelectionContext(t, &mock.FlapsClient{}, "-m", "machine-id")

		machineID, err := resolveMachineID(ctx, "test-app")

		require.NoError(t, err)
		assert.Equal(t, "machine-id", machineID)
	})

	t.Run("preserves the instance alias", func(t *testing.T) {
		ctx := machineSelectionContext(t, &mock.FlapsClient{}, "-i", "machine-id")
		ctx, err := preparers.ApplyAliases(ctx)
		require.NoError(t, err)

		machineID, err := resolveMachineID(ctx, "test-app")

		require.NoError(t, err)
		assert.Equal(t, "machine-id", machineID)
	})

	t.Run("rejects machine and select together", func(t *testing.T) {
		ctx := machineSelectionContext(t, &mock.FlapsClient{}, "--machine", "machine-id", "--select")

		_, err := resolveMachineID(ctx, "test-app")

		require.EqualError(t, err, "--machine can't be used with -s/--select")
	})

	t.Run("uses the only available machine", func(t *testing.T) {
		client := &mock.FlapsClient{
			ListActiveFunc: func(_ context.Context, appName string) ([]*fly.Machine, error) {
				assert.Equal(t, "test-app", appName)

				return []*fly.Machine{{ID: "machine-id", Region: "iad"}}, nil
			},
		}
		ctx := machineSelectionContext(t, client, "--select")

		machineID, err := resolveMachineID(ctx, "test-app")

		require.NoError(t, err)
		assert.Equal(t, "machine-id", machineID)
	})

	t.Run("reports when the app has no machines", func(t *testing.T) {
		client := &mock.FlapsClient{
			ListActiveFunc: func(_ context.Context, _ string) ([]*fly.Machine, error) {
				return nil, nil
			},
		}
		ctx := machineSelectionContext(t, client, "--select")

		_, err := resolveMachineID(ctx, "test-app")

		require.EqualError(t, err, "app test-app has no machines")
	})

	t.Run("prompts when multiple machines are available", func(t *testing.T) {
		client := &mock.FlapsClient{
			ListActiveFunc: func(_ context.Context, _ string) ([]*fly.Machine, error) {
				return []*fly.Machine{
					{ID: "machine-b", Region: "iad"},
					{ID: "machine-a", Region: "syd"},
				}, nil
			},
		}
		ctx := machineSelectionContext(t, client, "--select")

		_, err := resolveMachineID(ctx, "test-app")

		require.ErrorIs(t, err, prompt.ErrNonInteractive)
	})
}

func machineSelectionContext(t *testing.T, client flapsutil.FlapsClient, args ...string) context.Context {
	t.Helper()

	cmd := New()
	require.NoError(t, cmd.Flags().Parse(args))

	ctx := flag.NewContext(context.Background(), cmd.Flags())
	ios, _, _, _ := iostreams.Test()
	ctx = iostreams.NewContext(ctx, ios)

	return flapsutil.NewContextWithClient(ctx, client)
}
