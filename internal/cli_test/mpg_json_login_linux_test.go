package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/cli"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/sys/unix"
)

func TestMPGJSONLoginCLI(t *testing.T) {
	for _, operation := range []string{"create", "fork", "restore", "users create", "databases create"} {
		for _, mode := range []string{"env", "flag"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
				if err != nil {
					t.Skipf("no pseudo-terminal available: %v", err)
				}
				defer master.Close()
				require.NoError(t, unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0))
				number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
				require.NoError(t, err)
				slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
				require.NoError(t, err)
				defer slave.Close()
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMPGJSONLoginProcess$")
				cmd.Dir = t.TempDir()
				cmd.Env = append(os.Environ(), "FLYCTL_MPG_LOGIN_TEST="+operation, "FLYCTL_MPG_LOGIN_MODE="+mode)
				var stderr strings.Builder
				cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, &stderr
				output := make(chan []byte, 1)
				go func() {
					var data []byte
					buf := make([]byte, 4096)
					for {
						n, readErr := master.Read(buf)
						data = append(data, buf[:n]...)
						if n > 0 {
							cancel() // Any stdout violates JSON login's contract; stop a prompt immediately.
						}
						if readErr != nil {
							break
						}
					}
					output <- data
				}()
				err = cmd.Run()
				slave.Close()
				stdout := <-output
				require.Empty(t, string(stdout))
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "%s", stderr.String())
				require.Equal(t, 1, exit.ExitCode())
				require.Contains(t, stderr.String(), "Error: no access token available")
				require.NotContains(t, stderr.String(), "Would you like")
			})
		}
	}
}

func TestMPGJSONLoginProcess(t *testing.T) {
	operation := os.Getenv("FLYCTL_MPG_LOGIN_TEST")
	if operation == "" {
		t.Skip("subprocess helper")
	}
	http.DefaultTransport = mpgNoNetwork{}
	for _, key := range []string{"CI", "GITHUB_ACTIONS", "CONTINUOUS_INTEGRATION", "BUILD_NUMBER", "RUN_ID"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	for key, value := range map[string]string{
		"FLY_CONFIG_DIR":   filepath.Join(t.TempDir(), "config"),
		"FLY_ACCESS_TOKEN": "", "FLY_API_TOKEN": "", "FLY_JSON": "0", "FLY_NO_PROMPT": "",
		"FLY_NO_UPDATE_CHECK": "1", "FLY_UPDATE_CHECK": "0", "FLY_SEND_METRICS": "0",
		"FLY_NO_INCIDENTS_CHECK": "1", "FLY_INCIDENTS_CHECK": "0", "NO_COLOR": "1",
	} {
		t.Setenv(key, value)
	}
	args := append([]string{"mpg"}, strings.Fields(operation)...)
	if os.Getenv("FLYCTL_MPG_LOGIN_MODE") == "env" {
		t.Setenv("FLY_JSON", "1")
	} else {
		args = append(args, "--json")
	}
	ios := &iostreams.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	ctx := metrics.WithDisableFlushMetrics(context.Background())
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{AuthenticatedFunc: func() bool { return false }})
	code := cli.Run(ctx, ios, args...)
	os.Exit(code)
}
