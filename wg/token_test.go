package wg

import (
	"encoding/base64"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGatewayPubkey is a syntactically valid (32-byte) WireGuard key.
var testGatewayPubkey = base64.StdEncoding.EncodeToString(make([]byte, 32))

func fakeGateway(t *testing.T, conn net.Conn, result *tokenResult) chan tokenAuthPacket {
	return fakeGatewayHello(t, conn, result, false)
}

// fakeGatewayHello serves one exchange; authHeader is what the hello
// advertises about reading the Authorization header.
func fakeGatewayHello(t *testing.T, conn net.Conn, result *tokenResult, authHeader bool) chan tokenAuthPacket {
	t.Helper()

	got := make(chan tokenAuthPacket, 1)

	go func() {
		defer conn.Close()

		if err := writeJSONFrame(conn, &tokenHello{
			Version:    tokenProtoVersion,
			Type:       "hello",
			Pubkey:     testGatewayPubkey,
			AuthHeader: authHeader,
		}); err != nil {
			return
		}

		var auth tokenAuthPacket
		if err := readJSONFrame(conn, &auth); err != nil {
			return
		}
		got <- auth

		_ = writeJSONFrame(conn, result)
	}()

	return got
}

func TestTokenExchange(t *testing.T) {
	client, gateway := net.Pipe()
	defer client.Close()

	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	got := fakeGateway(t, gateway, &tokenResult{
		Type:      "ok",
		PeerIP:    "fdaa:0:18:ac10:5:1234:5678:9a02",
		DNS:       "fdaa:0:18::3",
		ExpiresAt: expires.Unix(),
	})

	prov, err := tokenExchange(client, &TokenAuth{
		Pubkey:  "CLIENT_PUBKEY",
		OrgSlug: "personal",
	}, "FlyV1 fm2_test")
	require.NoError(t, err)

	assert.Equal(t, testGatewayPubkey, prov.GatewayPubkey)
	assert.Equal(t, "fdaa:0:18:ac10:5:1234:5678:9a02", prov.PeerIP)
	assert.Equal(t, "fdaa:0:18::3", prov.DNS)
	assert.True(t, prov.ExpiresAt.Equal(expires))

	auth := <-got
	assert.Equal(t, tokenProtoVersion, auth.Version)
	assert.Equal(t, "auth", auth.Type)
	assert.Equal(t, "FlyV1 fm2_test", auth.Token)
	assert.Equal(t, "CLIENT_PUBKEY", auth.Pubkey)
	assert.Equal(t, "personal", auth.OrgSlug)
}

func TestTokenExchangeOmitsPacketTokenForHeaderGateway(t *testing.T) {
	client, gateway := net.Pipe()
	defer client.Close()

	got := fakeGatewayHello(t, gateway, &tokenResult{
		Type:   "ok",
		PeerIP: "fdaa:0:18:ac10:5:1234:5678:9a02",
	}, true)

	_, err := tokenExchange(client, &TokenAuth{Pubkey: "CLIENT_PUBKEY"}, "FlyV1 fm2_test")
	require.NoError(t, err)

	auth := <-got
	assert.Equal(t, "", auth.Token, "gateway reads the header; the packet must not carry a copy")
	assert.Equal(t, "CLIENT_PUBKEY", auth.Pubkey)
}

func TestTokenExchangeRejected(t *testing.T) {
	client, gateway := net.Pipe()
	defer client.Close()

	fakeGateway(t, gateway, &tokenResult{
		Type:  "error",
		Code:  "unauthorized",
		Error: "token not authorized for wireguard access",
	})

	_, err := tokenExchange(client, &TokenAuth{}, "FlyV1 fm2_bad")
	require.Error(t, err)

	var gwErr *GatewayError
	require.True(t, errors.As(err, &gwErr))
	assert.Equal(t, "unauthorized", gwErr.Code)
	assert.True(t, gwErr.Permanent())
}

func TestTokenExchangeMalformedReply(t *testing.T) {
	cases := map[string]tokenResult{
		"bad peer ip": {Type: "ok", PeerIP: "not-an-ip", DNS: "fdaa:0:18::3"},
		"ipv4 peer":   {Type: "ok", PeerIP: "10.0.0.2"},
		"bad dns":     {Type: "ok", PeerIP: "fdaa:0:18:ac10:5:1234:5678:9a02", DNS: "nope"},
	}

	for name, res := range cases {
		t.Run(name, func(t *testing.T) {
			client, gateway := net.Pipe()
			defer client.Close()

			fakeGateway(t, gateway, &res)

			_, err := tokenExchange(client, &TokenAuth{}, "FlyV1 fm2_test")
			assert.ErrorIs(t, err, ErrMalformedReply)
		})
	}

	t.Run("bad gateway pubkey", func(t *testing.T) {
		client, gateway := net.Pipe()
		defer client.Close()

		go func() {
			defer gateway.Close()
			_ = writeJSONFrame(gateway, &tokenHello{Version: tokenProtoVersion, Type: "hello", Pubkey: "short"})
			var auth tokenAuthPacket
			_ = readJSONFrame(gateway, &auth)
			_ = writeJSONFrame(gateway, &tokenResult{Type: "ok", PeerIP: "fdaa:0:18:ac10:5:1234:5678:9a02"})
		}()

		_, err := tokenExchange(client, &TokenAuth{}, "FlyV1 fm2_test")
		assert.ErrorIs(t, err, ErrMalformedReply)
	})
}

func TestTokenExchangeNoToken(t *testing.T) {
	client, gateway := net.Pipe()
	defer client.Close()
	defer gateway.Close()

	_, err := tokenExchange(client, &TokenAuth{}, "")
	assert.ErrorIs(t, err, ErrNoToken)
}

func TestTokenExchangeClosedBeforeHelloIsNotNoHello(t *testing.T) {
	client, gateway := net.Pipe()
	defer client.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		gateway.Close()
	}()

	_, err := tokenExchange(client, &TokenAuth{}, "FlyV1 fm2_test")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoHello)
}

func TestProvisionSame(t *testing.T) {
	a := &TokenProvision{PeerIP: "fdaa::2", GatewayPubkey: "k", DNS: "fdaa::3"}
	assert.True(t, a.same(&TokenProvision{PeerIP: "fdaa::2", GatewayPubkey: "k", DNS: "fdaa::3"}))
	assert.False(t, a.same(&TokenProvision{PeerIP: "fdaa::2", GatewayPubkey: "k", DNS: "fdaa::4"}))
	assert.False(t, a.same(&TokenProvision{PeerIP: "fdaa::9", GatewayPubkey: "k", DNS: "fdaa::3"}))
	assert.False(t, a.same(nil))
}

func TestTokenExchangeHelloTimeout(t *testing.T) {
	// tokenHelloTimeout is shortened in TestMain

	// a legacy gateway waits for our magic and never speaks first
	client, gateway := net.Pipe()
	defer client.Close()
	defer gateway.Close()

	start := time.Now()
	_, err := tokenExchange(client, &TokenAuth{}, "FlyV1 fm2_test")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNoHello)
	assert.Less(t, time.Since(start), 10*time.Second)
}

func TestTokenPeerName(t *testing.T) {
	// 32 bytes 0x01..0x20
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i + 1)
	}

	assert.Equal(t, "token-01020304", TokenPeerName(base64.StdEncoding.EncodeToString(raw)))
	assert.Equal(t, "token-peer", TokenPeerName("not base64!"))
}
