package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/iostreams"
)

func TestPrintWrappedErrorDetailsOnStderr(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	failure := fmt.Errorf("updating machine: %w", flyerr.GenericErr{
		Err:      "timeout reached waiting for machine's state to change",
		Descript: "The machine target took more than 1s to reach \"settled\"",
		Suggest:  "Try increasing the --wait-timeout",
	})
	printError(ios, ios.ColorScheme(), nil, failure)
	require.Empty(t, stdout.String())
	require.Equal(t, "Error: updating machine: timeout reached waiting for machine's state to change\nThe machine target took more than 1s to reach \"settled\"\n\nTry increasing the --wait-timeout\n\n", stderr.String())
}
