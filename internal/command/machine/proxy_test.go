package machine

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag"
)

// The Machines API proxy must not listen on every interface unless asked to.
func TestAPIProxyBindAddr(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"localhost by default", nil, "127.0.0.1"},
		{"flag", []string{"--bind-addr", "0.0.0.0"}, "0.0.0.0"},
		{"shorthand", []string{"-b", "::1"}, "::1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newProxy()
			require.NoError(t, cmd.ParseFlags(tc.args))
			params := apiProxyParams(flag.NewContext(t.Context(), cmd.Flags()), "personal", nil)
			require.Equal(t, tc.want, params.BindAddr)
			require.Equal(t, []string{"4280"}, params.Ports)
			require.Equal(t, "_api.internal", params.RemoteHost)
		})
	}
}
