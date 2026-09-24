package wg

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// Token-authenticated wswg protocol, speaking to wggwd gateways running in
// TOKEN_WSWG mode. Instead of provisioning a peer through web's GraphQL API,
// the client authenticates its websocket connection with a macaroon and the
// gateway allocates the peer inline:
//
//	gateway -> client   tokenHello  {version, pubkey}
//	client  -> gateway  tokenAuth   {version, token, pubkey, org_slug, network_name?}
//	gateway -> client   tokenResult {peer_ip, dns, expires_at} or {error, code}
//
// followed by the usual length-prefixed WireGuard frame relay. Control
// packets use the same 4-byte big-endian length framing, with JSON payloads.

const (
	tokenProtoVersion = 1
	maxControlFrame   = 64 * 1024

	// Gateway error codes. "unauthorized" and "invalid_request" won't be
	// fixed by reconnecting with the same credentials; "transient" might.
	GatewayErrUnauthorized   = "unauthorized"
	GatewayErrInvalidRequest = "invalid_request"
	GatewayErrTransient      = "transient"
)

var (
	// ErrNoHello means the gateway never started the token exchange,
	// which is what a legacy (non-token) gateway looks like.
	ErrNoHello = errors.New("no token-mode hello from gateway")

	// ErrNoToken means there was no macaroon to present.
	ErrNoToken = errors.New("no macaroon token to present to the gateway")

	// ErrMalformedReply means the gateway's answer didn't parse as a usable
	// provision (bad key or address); a protocol problem, not a network one.
	ErrMalformedReply = errors.New("malformed reply from gateway")

	// tokenHelloTimeout bounds how long we wait for the gateway to speak
	// first. A legacy (non-token) gateway never does: it waits for our
	// magic, so without this both sides would wait forever.
	tokenHelloTimeout = 15 * time.Second

	// tokenExchangeTimeout bounds the whole hello/auth/result exchange.
	tokenExchangeTimeout = 30 * time.Second
)

// TokenAuth is what the client presents to the gateway: a macaroon token
// header plus the peer it wants provisioned.
type TokenAuth struct {
	// Token returns the macaroon header to present. It's called on every
	// (re-)connection so that refreshed tokens are picked up.
	Token       func() string
	Pubkey      string
	OrgSlug     string
	NetworkName string
}

// TokenProvision is the gateway's answer: the peer address it allocated (and
// the gateway public key it announced in its hello packet).
type TokenProvision struct {
	GatewayPubkey string
	PeerIP        string
	DNS           string
	ExpiresAt     time.Time

	// token is the header this provision was granted for, so a refresh can
	// tell whether there's anything new to present
	token string
}

// same returns whether p and o describe the same peer on the same gateway
// with the same resolver. Anything else means the tunnel's addresses and
// remote key must change.
func (p *TokenProvision) same(o *TokenProvision) bool {
	return p != nil && o != nil && p.PeerIP == o.PeerIP && p.GatewayPubkey == o.GatewayPubkey && p.DNS == o.DNS
}

// validate rejects provisions the tunnel code couldn't build a device from
// (it panics on garbage keys and addresses, by design of TunnelConfig).
func (p *TokenProvision) validate() error {
	if raw, err := base64.StdEncoding.DecodeString(p.GatewayPubkey); err != nil || len(raw) != 32 {
		return fmt.Errorf("%w: gateway pubkey %q", ErrMalformedReply, p.GatewayPubkey)
	}

	if ip := net.ParseIP(p.PeerIP); ip == nil || ip.To4() != nil {
		return fmt.Errorf("%w: peer ip %q", ErrMalformedReply, p.PeerIP)
	}

	if p.DNS != "" {
		if ip := net.ParseIP(p.DNS); ip == nil || ip.To4() != nil {
			return fmt.Errorf("%w: dns %q", ErrMalformedReply, p.DNS)
		}
	}

	return nil
}

type tokenHello struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Pubkey  string `json:"pubkey"`
}

type tokenAuthPacket struct {
	Version     int    `json:"version"`
	Type        string `json:"type"`
	Token       string `json:"token"`
	Pubkey      string `json:"pubkey"`
	OrgSlug     string `json:"org_slug"`
	NetworkName string `json:"network_name,omitempty"`
}

type tokenResult struct {
	Type      string `json:"type"`
	PeerIP    string `json:"peer_ip,omitempty"`
	DNS       string `json:"dns,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Code      string `json:"code,omitempty"`
	Error     string `json:"error,omitempty"`
}

// GatewayError is a rejection from the gateway.
type GatewayError struct {
	Code    string
	Message string
}

func (e *GatewayError) Error() string {
	return fmt.Sprintf("gateway rejected connection: %s (%s)", e.Message, e.Code)
}

// Permanent reports whether reconnecting with the same credentials is
// pointless: the token was rejected or the request itself is invalid.
func (e *GatewayError) Permanent() bool {
	return e.Code == GatewayErrUnauthorized || e.Code == GatewayErrInvalidRequest
}

// TokenPeerName mirrors the name wggwd gives an inline-provisioned peer
// ("token-" + the first four key bytes in hex), so logs on both sides agree.
func TokenPeerName(pubkey string) string {
	raw, err := base64.StdEncoding.DecodeString(pubkey)
	if err != nil || len(raw) < 4 {
		return "token-peer"
	}

	return fmt.Sprintf("token-%x", raw[:4])
}

// deadlineHit reports whether a read failed because its deadline passed.
// A plain net.Conn says os.ErrDeadlineExceeded; the websocket-backed conn
// cancels the context behind an in-flight read instead, so its error wraps
// context.Canceled (or context.DeadlineExceeded when no read was active).
func deadlineHit(err error) bool {
	return errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		isTimeout(err)
}

func writeJSONFrame(w io.Writer, v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal control packet: %w", err)
	}

	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)

	_, err = w.Write(buf)
	return err
}

func readJSONFrame(r io.Reader, v any) error {
	var lenbuf [4]byte
	if _, err := io.ReadFull(r, lenbuf[:]); err != nil {
		return err
	}

	size := binary.BigEndian.Uint32(lenbuf[:])
	if size == 0 || size > maxControlFrame {
		return fmt.Errorf("control packet size %d out of bounds", size)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return err
	}

	return json.Unmarshal(payload, v)
}

// tokenExchange authenticates a freshly-dialed websocket connection and
// returns the peer the gateway provisioned for us.
func tokenExchange(conn net.Conn, auth *TokenAuth) (*TokenProvision, error) {
	start := time.Now()

	if err := conn.SetDeadline(start.Add(tokenHelloTimeout)); err != nil {
		return nil, err
	}
	defer conn.SetDeadline(time.Time{}) // skipcq: GO-S2307

	var hello tokenHello
	if err := readJSONFrame(conn, &hello); err != nil {
		// only the deadline counts as "no hello"; a closed conn mid-wait
		// is an ordinary read error
		if deadlineHit(err) && time.Since(start) >= tokenHelloTimeout {
			return nil, fmt.Errorf("%w within %s: is it a token-mode gateway?", ErrNoHello, tokenHelloTimeout)
		}

		return nil, fmt.Errorf("read gateway hello: %w", err)
	}
	if hello.Type != "hello" || hello.Version != tokenProtoVersion {
		return nil, fmt.Errorf("unexpected gateway hello (type %q version %d)", hello.Type, hello.Version)
	}

	if err := conn.SetDeadline(start.Add(tokenExchangeTimeout)); err != nil {
		return nil, err
	}

	var token string
	if auth.Token != nil {
		token = auth.Token()
	}
	if token == "" {
		return nil, ErrNoToken
	}

	err := writeJSONFrame(conn, &tokenAuthPacket{
		Version:     tokenProtoVersion,
		Type:        "auth",
		Token:       token,
		Pubkey:      auth.Pubkey,
		OrgSlug:     auth.OrgSlug,
		NetworkName: auth.NetworkName,
	})
	if err != nil {
		return nil, fmt.Errorf("write auth packet: %w", err)
	}

	var res tokenResult
	if err := readJSONFrame(conn, &res); err != nil {
		return nil, fmt.Errorf("read gateway result: %w", err)
	}

	switch res.Type {
	case "ok":
		prov := &TokenProvision{
			GatewayPubkey: hello.Pubkey,
			PeerIP:        res.PeerIP,
			DNS:           res.DNS,
			ExpiresAt:     time.Unix(res.ExpiresAt, 0),
			token:         token,
		}
		if err := prov.validate(); err != nil {
			return nil, err
		}

		return prov, nil
	case "error":
		return nil, &GatewayError{Code: res.Code, Message: res.Error}
	default:
		return nil, fmt.Errorf("unexpected gateway result type %q", res.Type)
	}
}
