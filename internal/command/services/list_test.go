package services

import (
	"context"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestListServicePorts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ports     []fly.MachinePort
		wantPorts []string
		wantErr   string
	}{
		{
			name:      "single port",
			ports:     []fly.MachinePort{{Port: lo.ToPtr(443)}},
			wantPorts: []string{"443"},
		},
		{
			name:      "port range",
			ports:     []fly.MachinePort{{StartPort: lo.ToPtr(1000), EndPort: lo.ToPtr(1005)}},
			wantPorts: []string{"1000-1005"},
		},
		{
			name:      "lower bound only",
			ports:     []fly.MachinePort{{StartPort: lo.ToPtr(1000)}},
			wantPorts: []string{"1000-65535"},
		},
		{
			name:      "upper bound only",
			ports:     []fly.MachinePort{{EndPort: lo.ToPtr(1005)}},
			wantPorts: []string{"0-1005"},
		},
		{
			name: "single ports and ranges stay distinct",
			ports: []fly.MachinePort{
				{Port: lo.ToPtr(443)},
				{StartPort: lo.ToPtr(1000), EndPort: lo.ToPtr(1005)},
			},
			wantPorts: []string{"443", "1000-1005"},
		},
		{
			name:    "missing port and range",
			ports:   []fly.MachinePort{{}},
			wantErr: "machine machine-iad has a service port without a port or range",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range tc.ports {
				tc.ports[i].Handlers = []string{"tls"}
				tc.ports[i].ForceHTTPS = true
			}
			client := &mock.FlapsClient{
				ListFunc: func(_ context.Context, app, state string) ([]*fly.Machine, error) {
					require.Equal(t, "test-app", app)
					require.Empty(t, state)
					var machines []*fly.Machine
					for _, region := range []string{"iad", "ord"} {
						machines = append(machines, &fly.Machine{
							ID:     "machine-" + region,
							State:  "started",
							Region: region,
							Config: &fly.MachineConfig{
								Metadata: map[string]string{"fly_process_group": "app"},
								Services: []fly.MachineService{{Protocol: "tcp", InternalPort: 8000, Ports: tc.ports}},
							},
						})
					}

					return machines, nil
				},
			}
			ios, _, out, _ := iostreams.Test()
			ctx := iostreams.NewContext(context.Background(), ios)
			ctx = flag.NewContext(ctx, newList().Flags())
			ctx = appconfig.WithName(ctx, "test-app")
			ctx = flapsutil.NewContextWithClient(ctx, client)

			err := runList(ctx)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)

				return
			}
			require.NoError(t, err)
			output := strings.Join(strings.Fields(strings.ReplaceAll(out.String(), "│", " ")), " ")
			for _, ports := range tc.wantPorts {
				require.Contains(t, output, "TCP "+ports+" => 8000 [TLS] True app iad,ord 2")
			}
		})
	}
}
