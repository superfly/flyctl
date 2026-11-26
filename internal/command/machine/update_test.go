package machine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunUpdateErrors(t *testing.T) {
	tests := []struct {
		name       string
		storedCPUs int
		flags      []string
		reason     string
		timeout    bool
	}{
		{name: "invalid CPUs", flags: []string{"--vm-cpus", "3"}, reason: "invalid number of CPUs"},
		{name: "invalid memory increment", flags: []string{"--vm-memory", "10"}, reason: "invalid memory size"},
		{name: "memory too low", storedCPUs: 4, flags: []string{"--vm-memory", "256"}, reason: "memory size for config is too low"},
		{name: "memory too high", storedCPUs: 4, flags: []string{"--vm-memory", "16384"}, reason: "memory size for config is too high"},
		{name: "wait deadline", flags: []string{"--wait-timeout", "1"}, timeout: true, reason: "timeout reached waiting for machine's state to change"},
		{name: "successful update", flags: []string{"--vm-memory", "512"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &fly.Machine{ID: "target", Version: "fixture-version", State: "stopped", Region: "ord", HostStatus: fly.HostStatusOk, Config: &fly.MachineConfig{Image: "example/image:latest", Guest: &fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 256}}}
			if tt.storedCPUs != 0 {
				source.Config.Guest.CPUs = tt.storedCPUs
				source.Config.Guest.MemoryMB = 2048
			}
			var updateStarted time.Time
			updates, waits, releases := 0, 0, 0
			client := &mock.FlapsClient{
				GetFunc: func(_ context.Context, app, id string) (*fly.Machine, error) {
					require.Equal(t, "update-errors", app)
					require.Equal(t, source.ID, id)

					return source, nil
				},
				AcquireLeaseFunc: func(context.Context, string, string, *int) (*fly.MachineLease, error) {
					return &fly.MachineLease{Data: &fly.MachineLeaseData{Nonce: "nonce"}}, nil
				},
				ReleaseLeaseFunc: func(_ context.Context, app, id, nonce string) error {
					releases++
					require.Equal(t, "nonce", nonce)

					return nil
				},
				UpdateFunc: func(_ context.Context, app string, input fly.LaunchMachineInput, nonce string) (*fly.Machine, error) {
					updates++
					updateStarted = time.Now()
					require.Equal(t, "update-errors", app)
					require.Equal(t, source.ID, input.ID)
					require.Equal(t, "nonce", nonce)
					if tt.reason == "" {
						require.Equal(t, 512, input.Config.Guest.MemoryMB)
					}

					return source, nil
				},
				WaitFunc: func(ctx context.Context, _ string, _ string, _ ...flaps.WaitOption) error {
					waits++
					if tt.timeout {
						deadline, ok := ctx.Deadline()
						require.True(t, ok)
						require.False(t, deadline.Before(updateStarted.Add(time.Second)))
						require.False(t, deadline.After(time.Now().Add(time.Second)))
						<-ctx.Done()

						return ctx.Err()
					}

					return nil
				},
			}
			cmd := newUpdate()
			require.NoError(t, cmd.ParseFlags(append([]string{source.ID, "--yes"}, tt.flags...)))
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(t.Context(), ios)
			ctx = flag.NewContext(ctx, cmd.Flags())
			ctx = appconfig.WithName(ctx, "update-errors")
			ctx = flapsutil.NewContextWithClient(ctx, client)
			err := runUpdate(ctx)
			require.Equal(t, 1, releases)
			if tt.reason == "" {
				require.NoError(t, err)
				require.Equal(t, 1, updates)
				require.Equal(t, 1, waits)
				require.Contains(t, out.String(), "updated successfully!")

				return
			}
			require.EqualError(t, err, tt.reason)
			require.NotContains(t, out.String(), "updated successfully!")
			if tt.timeout {
				var converted flyerr.GenericErr
				require.ErrorAs(t, err, &converted)
				require.Contains(t, converted.Descript, "more than 1s to reach \"settled\"")
				require.Contains(t, converted.Suggest, "--wait-timeout")
				require.Equal(t, 1, updates)
				require.Equal(t, 1, waits)
			} else {
				require.Zero(t, updates)
				require.Zero(t, waits)
			}
		})
	}
}
