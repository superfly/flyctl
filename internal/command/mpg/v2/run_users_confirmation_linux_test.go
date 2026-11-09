package cmdv2

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/sys/unix"
)

// Exercise the real Survey prompt through a terminal, including decline and
// acceptance when --username is supplied, before any mutation.
func TestRunUsersRotatePasswordInteractiveConfirmation(t *testing.T) {
	for _, answer := range []string{"n", "y"} {
		t.Run(answer, func(t *testing.T) {
			master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
			require.NoError(t, err)
			defer master.Close()
			require.NoError(t, unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0))
			number, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
			require.NoError(t, err)
			slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|unix.O_NOCTTY, 0)
			require.NoError(t, err)
			defer slave.Close()
			ctx, _, stderr := usersTestContext(t, false)
			io := iostreams.FromContext(ctx)
			io.In, io.Out = slave, slave
			io.SetStdinTTY(true)
			io.SetStdoutTTY(true)
			require.NoError(t, flag.SetString(ctx, "username", "app_user"))
			calls := 0
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{RotateManagedPostgresUserPasswordFunc: func(context.Context, string, string, flaps.RotateManagedPostgresUserPasswordRequest) (flaps.ManagedPostgresUserCredentials, error) {
				calls++
				return flaps.ManagedPostgresUserCredentials{Username: "app_user", Password: "new-password"}, nil
			}})
			done := make(chan error, 1)
			go func() { done <- RunUsersRotatePassword(ctx, "mpg-123") }()
			promptOutput := make(chan string, 1)
			go func() {
				var output strings.Builder
				buf := make([]byte, 4096)
				answered := false
				for {
					n, err := master.Read(buf)
					chunk := string(buf[:n])
					output.WriteString(chunk)
					for j := 0; j < strings.Count(chunk, "\x1b[6n"); j++ {
						_, _ = master.Write([]byte("\x1b[1;1R"))
					}
					if !answered && strings.Contains(output.String(), "Rotate password for user app_user") && strings.Contains(chunk, "\x1b[6n") {
						// Survey cursor reporting may buffer trailing input; answer after its
						// response has been consumed instead of sending both together.
						time.Sleep(50 * time.Millisecond)
						_, _ = master.Write([]byte(answer + "\r"))
						answered = true
						promptOutput <- output.String()
					}
					if err != nil {
						if !answered {
							promptOutput <- output.String()
						}
						return
					}
				}
			}()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(10 * time.Second):
				master.Close()
				slave.Close()
				t.Fatal("confirmation did not complete")
			}
			require.Contains(t, <-promptOutput, "Rotate password for user app_user")
			if answer == "n" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			require.Contains(t, stderr.String(), "DATABASE_URL")
		})
	}
}
