package wg

import (
	"fmt"
	"net"

	"github.com/superfly/flyctl/api"
)

type WireGuardState struct {
	Org          string                   `json:"org"`
	Name         string                   `json:"name"`
	Region       string                   `json:"region"`
	LocalPublic  string                   `json:"localprivate"`
	LocalPrivate string                   `json:"localpublic"`
	DNS          string                   `json:"dns"`
	Peer         api.CreatedWireGuardPeer `json:"peer"`
}

// BUG(tqbf): Obviously all this needs to go, and I should just
// make my code conform to the marshal/unmarshal protocol wireguard-go
// uses, but in the service of landing this feature, I'm just going
// to apply a layer of spackle for now.
func (s *WireGuardState) TunnelConfig() *Config {
	skey := PrivateKey{}
	if err := skey.UnmarshalText([]byte(s.LocalPrivate)); err != nil {
		panic(fmt.Sprintf("martian local private key: %s", err))
	}

	pkey := PublicKey{}
	if err := pkey.UnmarshalText([]byte(s.Peer.Pubkey)); err != nil {
		panic(fmt.Sprintf("martian local public key: %s", err))
	}

	_, lnet, err := net.ParseCIDR(fmt.Sprintf("%s/120", s.Peer.Peerip))
	if err != nil {
		panic(fmt.Sprintf("martian local public: %s/120: %s", s.Peer.Peerip, err))
	}

	raddr := net.ParseIP(s.Peer.Peerip).To16()
	for i := 6; i < 16; i++ {
		raddr[i] = 0
	}

	// BUG(tqbf): for now, we never manage tunnels for different
	// organizations, and while this comment is eating more space
	// than the code I'd need to do this right, it's more fun to
	// type, so we just hardcode.
	_, rnet, _ := net.ParseCIDR(fmt.Sprintf("%s/48", raddr))

	raddr[15] = 3
	dns := net.ParseIP(raddr.String())

	// BUG(tqbf): I think this dance just because these needed to
	// parse for Ben's TOML code.
	wgl := IPNet(*lnet)
	wgr := IPNet(*rnet)

	return &Config{
		LocalPrivateKey: skey,
		LocalNetwork:    &wgl,
		RemotePublicKey: pkey,
		RemoteNetwork:   &wgr,
		Endpoint:        s.Peer.Endpointip + ":51820",
		DNS:             dns,
		// LogLevel:        9999999,
	}
}
