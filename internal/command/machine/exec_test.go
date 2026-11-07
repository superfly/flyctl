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

func TestMachineExecReportsOutputFailures(t *testing.T) {
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
					require.ErrorIs(t, err, writeErr)
					if exitCode != 0 {
						require.ErrorContains(t, err, "exited with status 42")
					} else {
						require.ErrorContains(t, err, "completed successfully")
					}
					if !jsonOutput {
						if stream == "stdout" {
							require.Equal(t, "diagnostic\n", stderr.String())
						} else {
							require.Equal(t, "output\n", stdout.String())
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

func TestMachineExecReportsRemoteFailure(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, response := range []*fly.MachineExecResponse{
			{},
			{ExitCode: 42},
			{StdOut: "command output\n", StdErr: "command diagnostic\n"},
			{ExitCode: 42, StdOut: "command output\n", StdErr: "command diagnostic\n"},
		} {
			t.Run(fmt.Sprintf("json=%t/exit=%d/empty=%t", jsonOutput, response.ExitCode, response.StdOut == ""), func(t *testing.T) {
				cmd := newMachineExec()
				require.NoError(t, cmd.ParseFlags([]string{"test-machine", "command"}))
				ios, _, stdout, stderr := iostreams.Test()
				ctx := flag.NewContext(context.Background(), cmd.Flags())
				ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
				ctx = appconfig.WithName(ctx, "test-app")
				ctx = iostreams.NewContext(ctx, ios)
				ctx = flapsutil.NewContextWithClient(ctx, execTestClient{response: response})

				err := runMachineExec(ctx)
				if response.ExitCode == 0 {
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

func TestMachineExecPreservesBothOutputFailures(t *testing.T) {
	cmd := newMachineExec()
	require.NoError(t, cmd.ParseFlags([]string{"test-machine", "command"}))
	ios, _, _, _ := iostreams.Test()
	stdoutErr := errors.New("stdout unavailable")
	stderrErr := errors.New("stderr unavailable")
	ios.Out = execOutputErrorWriter{stdoutErr}
	ios.ErrOut = execOutputErrorWriter{stderrErr}
	ctx := flag.NewContext(context.Background(), cmd.Flags())
	ctx = config.NewContext(ctx, &config.Config{})
	ctx = appconfig.WithName(ctx, "test-app")
	ctx = iostreams.NewContext(ctx, ios)
	ctx = flapsutil.NewContextWithClient(ctx, execTestClient{response: &fly.MachineExecResponse{
		ExitCode: 42, StdOut: "output", StdErr: "diagnostic",
	}})

	err := runMachineExec(ctx)
	require.ErrorIs(t, err, stdoutErr)
	require.ErrorIs(t, err, stderrErr)
	require.ErrorContains(t, err, "exited with status 42")
}
