package cmdv1

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

func TestMaybeWarnNotReady(t *testing.T) {
	const name = "test-cluster"
	tests := []struct {
		name     string
		status   string
		wantWarn bool
	}{
		{name: "ready silent", status: "ready", wantWarn: false},
		{name: "creating warns", status: "creating", wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cluster := &mpgv1.ManagedCluster{Name: name, Status: tt.status}
			maybeWarnNotReady(&buf, cluster)

			got := buf.String()
			if !tt.wantWarn {
				require.Empty(t, got, "no warning expected for status=%q", tt.status)

				return
			}
			// Check the warning text independently of ANSI color codes.
			require.Contains(t, got, "WARN", "warning must contain the literal 'WARN' marker")
			require.Contains(t, got, "Cluster is not in ready state, currently: "+tt.status)
			require.True(t, bytes.HasSuffix(buf.Bytes(), []byte("\n")), "warning must end with a newline (pre-migration format)")
		})
	}
}
