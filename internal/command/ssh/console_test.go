package ssh

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
)

func sshFlagContext(t *testing.T, args ...string) context.Context {
	t.Helper()

	fs := pflag.NewFlagSet("ssh", pflag.ContinueOnError)
	fs.String("container", "", "")
	fs.Bool("no-container", false, "")
	fs.Bool("select", false, "")

	require.NoError(t, fs.Parse(args))

	return flag.NewContext(context.Background(), fs)
}

func TestSelectContainer(t *testing.T) {
	withContainers := &fly.Machine{
		ID: "3d8d9d16f14683",
		Config: &fly.MachineConfig{
			Containers: []*fly.ContainerConfig{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	}

	withoutContainers := &fly.Machine{
		ID:     "9080e6e2f24d83",
		Config: &fly.MachineConfig{},
	}
	// Config is nil when the host isn't reporting ok; IncompleteConfig still
	// carries the containers.
	withIncompleteConfig := &fly.Machine{
		ID:         "4d891de2f66587",
		HostStatus: fly.HostStatusUnknown,
		IncompleteConfig: &fly.MachineConfig{
			Containers: []*fly.ContainerConfig{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	}
	withoutConfig := &fly.Machine{ID: "e2865013b10d86"}

	for _, tc := range []struct {
		name      string
		machine   *fly.Machine
		args      []string
		expect    string
		expectErr string
	}{
		{
			name:    "defaults to the first container",
			machine: withContainers,
			expect:  "app",
		},
		{
			name:    "named container",
			machine: withContainers,
			args:    []string{"--container", "sidecar"},
			expect:  "sidecar",
		},
		{
			// The machine itself, so there is no container to select.
			name:    "no container",
			machine: withContainers,
			args:    []string{"--no-container"},
			expect:  "",
		},
		{
			name:      "no container with a named container",
			machine:   withContainers,
			args:      []string{"--no-container", "--container", "app"},
			expectErr: "--container and --no-container are mutually exclusive",
		},
		{
			name:    "no container on a machine without containers",
			machine: withoutContainers,
			args:    []string{"--no-container"},
			expect:  "",
		},
		{
			name:      "unknown container",
			machine:   withContainers,
			args:      []string{"--container", "nope"},
			expectErr: "container named nope is not present in machine 3d8d9d16f14683, try running with --select to see a list",
		},
		{
			name:    "incomplete configuration defaults to the first container",
			machine: withIncompleteConfig,
			expect:  "app",
		},
		{
			name:    "named container from incomplete configuration",
			machine: withIncompleteConfig,
			args:    []string{"--container", "sidecar"},
			expect:  "sidecar",
		},
		{
			name:      "unknown container in incomplete configuration",
			machine:   withIncompleteConfig,
			args:      []string{"--container", "nope"},
			expectErr: "container named nope is not present in machine 4d891de2f66587, try running with --select to see a list",
		},
		{
			// Nothing is known about the containers, so connect to the machine.
			name:    "missing configuration",
			machine: withoutConfig,
			expect:  "",
		},
		{
			// The name can't be checked, so it's passed through to the SSH server.
			name:    "named container with missing configuration",
			machine: withoutConfig,
			args:    []string{"--container", "app"},
			expect:  "app",
		},
		{
			name:    "no container with missing configuration",
			machine: withoutConfig,
			args:    []string{"--no-container"},
			expect:  "",
		},
		{
			name:      "conflicting container flags with missing configuration",
			machine:   withoutConfig,
			args:      []string{"--no-container", "--container", "app"},
			expectErr: "--container and --no-container are mutually exclusive",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := sshFlagContext(t, tc.args...)

			container, err := selectContainer(ctx, tc.machine)
			if tc.expectErr != "" {
				require.EqualError(t, err, tc.expectErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expect, container)
		})
	}
}
