package server

import (
	"crypto/x509"
	"errors"
	"net"
	"time"

	"github.com/superfly/flyctl/wg"
)

// Failure reason classes reported in the agent_wireguard/transport_failure
// metric. Gateway rejections use the gateway's own code (unauthorized,
// invalid_request, transient).
const (
	reasonNoToken   = "no_token"
	reasonMalformed = "malformed_reply"
	reasonNoHello   = "no_hello"
	reasonTLS       = "tls"
	reasonDial      = "dial"
	reasonRebuild   = "rebuild"
	reasonOther     = "other"
)

// failureReason classifies why a tunnel couldn't be established or died.
func failureReason(err error) string {
	var (
		gwErr      *wg.GatewayError
		rebuildErr *wg.RebuildError
		hostErr    x509.HostnameError
		authErr    x509.UnknownAuthorityError
		certErr    x509.CertificateInvalidError
		netErr     net.Error
	)

	switch {
	case errors.As(err, &gwErr):
		switch gwErr.Code {
		case wg.GatewayErrUnauthorized, wg.GatewayErrInvalidRequest, wg.GatewayErrTransient:
			return gwErr.Code
		default:
			return reasonOther
		}
	case errors.As(err, &rebuildErr):
		return reasonRebuild
	case errors.Is(err, wg.ErrNoToken):
		return reasonNoToken
	case errors.Is(err, wg.ErrMalformedReply):
		return reasonMalformed
	case errors.Is(err, wg.ErrNoHello):
		return reasonNoHello
	case errors.As(err, &hostErr), errors.As(err, &authErr), errors.As(err, &certErr):
		return reasonTLS
	case errors.As(err, &netErr):
		return reasonDial
	default:
		return reasonOther
	}
}

// unexpectedTokenFailure reports whether a token-mode failure points at a
// client or gateway problem worth a Sentry event, as opposed to the user's
// token aging out, a legacy gateway, or a network blip:
//
//   - the device couldn't be rebuilt after an anycast re-provision (a bug)
//   - the gateway's reply didn't parse (a protocol mismatch)
//   - the gateway called the request invalid, although the API had just
//     confirmed the org (a protocol mismatch, e.g. version skew)
//   - the token was rejected well before the expiry the gateway itself
//     stated for it (revocation, or a gateway-side verification problem)
//
// prov is the last successful provision, nil when establishing.
func unexpectedTokenFailure(reason string, prov *wg.TokenProvision, now time.Time) bool {
	switch reason {
	case reasonRebuild, reasonMalformed, wg.GatewayErrInvalidRequest:
		return true
	case wg.GatewayErrUnauthorized:
		// The proxy re-presents the token 45s before expiry; allow for that
		// and for clock skew before calling a rejection early.
		return prov != nil && now.Before(prov.ExpiresAt.Add(-unauthorizedGrace))
	default:
		return false
	}
}

const unauthorizedGrace = 90 * time.Second
