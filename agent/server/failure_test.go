package server

import (
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/wg"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestFailureReason(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"gateway unauthorized": {
			err:  fmt.Errorf("token-mode WireGuard via gw: %w", &wg.GatewayError{Code: wg.GatewayErrUnauthorized}),
			want: "unauthorized",
		},
		"gateway invalid request": {
			err:  &wg.GatewayError{Code: wg.GatewayErrInvalidRequest},
			want: "invalid_request",
		},
		"rebuild": {
			err:  &wg.RebuildError{PeerIP: "fdaa::2", Err: errors.New("boom")},
			want: reasonRebuild,
		},
		"no token": {
			err:  fmt.Errorf("token exchange: %w", wg.ErrNoToken),
			want: reasonNoToken,
		},
		"malformed reply": {
			err:  fmt.Errorf("token exchange: %w: peer ip \"x\"", wg.ErrMalformedReply),
			want: reasonMalformed,
		},
		"unknown gateway code": {
			err:  &wg.GatewayError{Code: ""},
			want: reasonOther,
		},
		"no hello": {
			err:  fmt.Errorf("token exchange: %w within 15s", wg.ErrNoHello),
			want: reasonNoHello,
		},
		"tls hostname": {
			err:  fmt.Errorf("websocket: %w", x509.HostnameError{Host: "gw"}),
			want: reasonTLS,
		},
		"dial": {
			err:  fmt.Errorf("websocket: %w", net.Error(timeoutErr{})),
			want: reasonDial,
		},
		"other": {
			err:  errors.New("martian"),
			want: reasonOther,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, failureReason(tc.err))
		})
	}
}

func TestUnexpectedTokenFailure(t *testing.T) {
	now := time.Now()
	fresh := &wg.TokenProvision{ExpiresAt: now.Add(10 * time.Minute)}
	expiring := &wg.TokenProvision{ExpiresAt: now.Add(45 * time.Second)}
	expired := &wg.TokenProvision{ExpiresAt: now.Add(-time.Minute)}

	assert.True(t, unexpectedTokenFailure(reasonRebuild, fresh, now))
	assert.True(t, unexpectedTokenFailure(reasonMalformed, nil, now))
	assert.True(t, unexpectedTokenFailure(wg.GatewayErrInvalidRequest, nil, now))

	// revoked, or the gateway can't verify a token it said was good
	assert.True(t, unexpectedTokenFailure(wg.GatewayErrUnauthorized, fresh, now))
	// the token aged out, or the pre-expiry re-presentation was refused
	assert.False(t, unexpectedTokenFailure(wg.GatewayErrUnauthorized, expiring, now))
	assert.False(t, unexpectedTokenFailure(wg.GatewayErrUnauthorized, expired, now))
	// rejected at establish: nothing to compare against
	assert.False(t, unexpectedTokenFailure(wg.GatewayErrUnauthorized, nil, now))

	for _, reason := range []string{wg.GatewayErrTransient, reasonNoHello, reasonTLS, reasonDial, reasonNoToken, reasonOther} {
		assert.False(t, unexpectedTokenFailure(reason, fresh, now), reason)
	}
}
