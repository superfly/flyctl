package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

type execTestClient struct {
	flapsutil.FlapsClient
	response *fly.MachineExecResponse
}

func (c execTestClient) Get(context.Context, string, string) (*fly.Machine, error) {
	return &fly.Machine{ID: "test-machine"}, nil
}

func (c execTestClient) Exec(context.Context, string, string, *fly.MachineExecRequest) (*fly.MachineExecResponse, error) {
	return c.response, nil
}

func TestMachineExecReportsRemoteFailure(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, exitCode := range []int32{0, 42} {
			t.Run(fmt.Sprintf("json=%t/exit=%d", jsonOutput, exitCode), func(t *testing.T) {
				cmd := newMachineExec()
				require.NoError(t, cmd.ParseFlags([]string{"test-machine", "command"}))
				ios, _, stdout, stderr := iostreams.Test()
				ctx := flag.NewContext(context.Background(), cmd.Flags())
				ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
				ctx = appconfig.WithName(ctx, "test-app")
				ctx = iostreams.NewContext(ctx, ios)
				response := &fly.MachineExecResponse{ExitCode: exitCode, StdOut: "output\n", StdErr: "diagnostic\n"}
				ctx = flapsutil.NewContextWithClient(ctx, execTestClient{response: response})

				err := runMachineExec(ctx)
				if exitCode == 0 {
					require.NoError(t, err)
				} else {
					require.EqualError(t, err, "command on machine test-machine exited with status 42")
				}
				if jsonOutput {
					var got fly.MachineExecResponse
					require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
					require.Equal(t, *response, got)
					require.Empty(t, stderr.String())
				} else {
					require.Equal(t, response.StdOut, stdout.String())
					require.Equal(t, response.StdErr, stderr.String())
				}
			})
		}
	}
}
