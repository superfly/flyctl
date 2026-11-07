package postgres

import (
	"context"
	"errors"
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

// Failover is unexpected in these cases; any dialer call must fail the test.
type restartTestDialer struct{ agent.Dialer }

func TestMachinesRestartWithoutLeader(t *testing.T) {
	for _, tt := range []struct {
		name       string
		force      bool
		hasLeader  bool
		single     bool
		restartErr error
		wantErr    string
	}{
		{name: "forced", force: true},
		{name: "forced single machine", force: true, single: true},
		{name: "not forced", wantErr: "no active leader found"},
		{name: "restart failure", force: true, restartErr: errors.New("restart failed"), wantErr: "restart failed"},
		{name: "leader present", hasLeader: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			makeMachine := func(id, role, region string) *fly.Machine {
				return &fly.Machine{
					ID:         id,
					State:      fly.MachineStateStarted,
					Region:     region,
					HostStatus: fly.HostStatusOk,
					Config:     &fly.MachineConfig{},
					ImageRef:   fly.MachineImageRef{Labels: map[string]string{"fly.version": "custom"}},
					Checks:     []*fly.MachineCheckStatus{{Name: "role", Status: fly.Passing, Output: role}},
				}
			}
			machines := []*fly.Machine{makeMachine("m1", "readonly", "iad")}
			if tt.hasLeader {
				machines = append(machines, makeMachine("m2", "primary", "lhr"))
			} else if !tt.single {
				machines = append(machines, makeMachine("m2", "readonly", "lhr"))
			}

			var restarted []string
			flapsClient := &mock.FlapsClient{
				ListFunc: func(ctx context.Context, appName, state string) ([]*fly.Machine, error) {
					return machines, nil
				},
				AcquireLeaseFunc: func(ctx context.Context, appName, machineID string, ttl *int) (*fly.MachineLease, error) {
					return &fly.MachineLease{Data: &fly.MachineLeaseData{Nonce: "test-nonce"}}, nil
				},
				RestartFunc: func(ctx context.Context, appName string, in fly.RestartMachineInput, nonce string) error {
					assert.Equal(t, "test-nonce", nonce)
					restarted = append(restarted, in.ID)
					return tt.restartErr
				},
				WaitFunc: func(ctx context.Context, appName, machineID string, opts ...flaps.WaitOption) error {
					return nil
				},
				ReleaseLeaseFunc: func(ctx context.Context, appName, machineID, nonce string) error {
					// Lease releases can run concurrently.
					return nil
				},
			}
			io, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), io)
			ctx = flapsutil.NewContextWithClient(ctx, flapsClient)
			ctx = agent.DialerWithContext(ctx, restartTestDialer{})
			flags := pflag.NewFlagSet("restart", pflag.ContinueOnError)
			flags.Bool("force", tt.force, "")
			ctx = flagctx.NewContext(ctx, flags)

			err := machinesRestart(ctx, "pg-app", &fly.RestartMachineInput{SkipHealthChecks: true})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				if !tt.force {
					assert.Empty(t, restarted)
				} else {
					assert.Len(t, restarted, 1)
				}

				return
			}
			require.NoError(t, err)
			wantRestarted := []string{"m1"}
			if !tt.single {
				wantRestarted = append(wantRestarted, "m2")
			}
			assert.ElementsMatch(t, wantRestarted, restarted)
			if tt.hasLeader {
				assert.Equal(t, "m2", restarted[len(restarted)-1])
			} else {
				assert.Contains(t, out.String(), "No leader found, but continuing with restart")
			}
			assert.Contains(t, out.String(), "Postgres cluster has been successfully restarted!")
		})
	}
}
