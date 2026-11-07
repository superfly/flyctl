package machine

import (
	"context"
	"encoding/json"
	"errors"
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

type execOutputErrorWriter struct{ err error }

func (w execOutputErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestMachineExecPreservesLegacyOutputFailureBehavior(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, exitCode := range []int32{0, 42} {
			for _, stream := range []string{"stdout", "stderr"} {
				if jsonOutput && stream == "stderr" {
					continue // JSON carries both remote streams on stdout.
				}
				t.Run(fmt.Sprintf("json=%t/exit=%d/%s", jsonOutput, exitCode, stream), func(t *testing.T) {
					cmd := newMachineExec()
					require.NoError(t, cmd.ParseFlags([]string{"test-machine", "command"}))
					ios, _, stdout, stderr := iostreams.Test()
					writeErr := errors.New("output unavailable")
					if stream == "stdout" {
						ios.Out = execOutputErrorWriter{writeErr}
					} else {
						ios.ErrOut = execOutputErrorWriter{writeErr}
					}
					ctx := flag.NewContext(context.Background(), cmd.Flags())
					ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
					ctx = appconfig.WithName(ctx, "test-app")
					ctx = iostreams.NewContext(ctx, ios)
					ctx = flapsutil.NewContextWithClient(ctx, execTestClient{response: &fly.MachineExecResponse{
						ExitCode: exitCode, StdOut: "output\n", StdErr: "diagnostic\n",
					}})

					err := runMachineExec(ctx)
					if jsonOutput {
						require.ErrorIs(t, err, writeErr)
					} else {
						require.NoError(t, err, "text-mode write errors must retain their existing behavior")
					}
					if !jsonOutput {
						if stream == "stdout" {
							require.Equal(t, "diagnostic\n", stderr.String())
						} else {
							want := "output\n"
							if exitCode != 0 {
								want = "Exit code: 42\n" + want
							}
							require.Equal(t, want, stdout.String())
						}
					}
				})
			}
		}
	}
}

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

func TestMachineExecPreservesLegacyRemoteStatusBehavior(t *testing.T) {
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
				require.NoError(t, err, "a remote failure must not change flyctl exit status in this PR")
				if jsonOutput {
					var got fly.MachineExecResponse
					require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
					require.Equal(t, *response, got)
					require.Empty(t, stderr.String())
				} else {
					want := response.StdOut
					if exitCode != 0 {
						want = "Exit code: 42\n" + want
					}
					require.Equal(t, want, stdout.String())
					require.Equal(t, response.StdErr, stderr.String())
				}
			})
		}
	}
}
