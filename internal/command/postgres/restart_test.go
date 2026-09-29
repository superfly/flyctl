package postgres

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

// stubDialer satisfies agent.Dialer; every method panics if called.
type stubDialer struct{ agent.Dialer }

func TestMachinesRestartForceWithoutLeader(t *testing.T) {
	t.Setenv("FLY_DEV", "1")

	// A single machine whose role check is not "leader" or "primary".
	machine := &fly.Machine{
		ID:         "m1",
		State:      fly.MachineStateStarted,
		Region:     "iad",
		HostStatus: fly.HostStatusUnreachable,
		Config:     &fly.MachineConfig{},
		Checks: []*fly.MachineCheckStatus{
			{Name: "role", Status: fly.Passing, Output: "readonly"},
		},
	}

	var restarted []string
	flapsClient := &mock.FlapsClient{
		ListFunc: func(ctx context.Context, appName, state string) ([]*fly.Machine, error) {
			return []*fly.Machine{machine}, nil
		},
		RestartFunc: func(ctx context.Context, appName string, in fly.RestartMachineInput, nonce string) error {
			restarted = append(restarted, in.ID)
			return nil
		},
		WaitFunc: func(ctx context.Context, appName, machineID string, opts ...flaps.WaitOption) error {
			return nil
		},
		ReleaseLeaseFunc: func(ctx context.Context, appName, machineID, nonce string) error {
			return nil
		},
	}

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
	// The dialer is only used to fail over a leader, which this test has none of.
	ctx = agent.DialerWithContext(ctx, stubDialer{})

	flags := pflag.NewFlagSet("restart", pflag.ContinueOnError)
	flags.Bool("force", true, "")
	ctx = flagctx.NewContext(ctx, flags)

	var err error
	require.NotPanics(t, func() {
		err = machinesRestart(ctx, "pg-app", &fly.RestartMachineInput{SkipHealthChecks: true})
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"m1"}, restarted)
}
