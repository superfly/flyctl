package sentry

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestIgnore(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"cancelled", fmt.Errorf("deploy: %w", context.Canceled), true},
		{"ctrl-c through context.Cause", errors.New(`Post "https://api.fly.io/api/v1/builds": interrupt signal received`), true},
		{"sigterm through context.Cause", errors.New(`Post "https://api.fly.io/api/v1/releases": terminated signal received`), true},
		{"remote exit status", fmt.Errorf("ssh shell: %w", &ssh.ExitError{}), true},
		{"machine limit", errors.New("Your organization has reached its machine limit. Please contact billing@fly.io"), true},
		{"overdue invoices", errors.New("failed to create release (status 403): Your account has overdue invoices."), true},
		{"not authorized is still reported", errors.New("failed to run create_wg_peers: Not authorized to manage this organization::feature."), false},
		{"user app crashing", errors.New("smoke checks for d89 failed: the app appears to be crashing"), true},
		{"nil pointer", errors.New("runtime error: invalid memory address or nil pointer dereference"), false},
		{"api timeout", errors.New(`Error contacting Fly.io API when probing "personal": timed out (context deadline exceeded)`), false},
		{"handshake", errors.New("error connecting to SSH server: ssh: handshake failed: EOF"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ignore(tc.err))
		})
	}
}
