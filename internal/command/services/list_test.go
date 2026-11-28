package services

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestListShowsPortRanges(t *testing.T) {
	machine := &fly.Machine{ID: "machine", Region: "ord", State: "started", Config: &fly.MachineConfig{
		Services: []fly.MachineService{{
			Protocol:     "tcp",
			InternalPort: 8080,
			Ports: []fly.MachinePort{
				{Port: new(443), Handlers: []string{"tls", "http"}, ForceHTTPS: true},
				{StartPort: new(9000), EndPort: new(9100), Handlers: []string{"tls"}},
				{StartPort: new(10000)},
				{EndPort: new(20)},
				{},
			},
		}},
	}}
	client := &mock.FlapsClient{
		ListFunc: func(context.Context, string, string) ([]*fly.Machine, error) {
			return []*fly.Machine{machine}, nil
		},
	}
	io, _, out, _ := iostreams.Test()
	ctx := iostreams.NewContext(t.Context(), io)
	ctx = flag.NewContext(ctx, pflag.NewFlagSet("test", pflag.ContinueOnError))
	ctx = appconfig.WithName(ctx, "test-app")
	ctx = flapsutil.NewContextWithClient(ctx, client)

	require.NoError(t, runList(ctx))
	for _, want := range []string{
		`443 => 8080\s.*\[TLS,HTTP\].*True`,
		`9000-9100 => 8080\s.*\[TLS\].*False`,
		`10000- => 8080\s.*\[\].*False`,
		`-20 => 8080\s.*\[\].*False`,
		`TCP\s.*- => 8080\s.*\[\].*False`,
	} {
		require.Regexp(t, want, out.String())
	}
}
