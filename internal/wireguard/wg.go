package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"

	badrand "math/rand"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/pkg/wg"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/crypto/curve25519"
)

func StateForOrg(apiClient *api.Client, org *api.Organization, regionCode string, name string) (*wg.WireGuardState, error) {
	state, err := getWireGuardStateForOrg(org.Slug)
	if err != nil {
		return nil, err
	}
	if state != nil {
		return state, nil
	}

	terminal.Debugf("Can't find matching WireGuard configuration; creating new one\n")

	stateb, err := Create(apiClient, org, regionCode, name)
	if err != nil {
		return nil, err
	}

	if err := setWireGuardStateForOrg(org.Slug, stateb); err != nil {
		return nil, err
	}

	return stateb, nil
}

func Create(apiClient *api.Client, org *api.Organization, regionCode, name string) (*wg.WireGuardState, error) {
	var (
		err error
		rx  = regexp.MustCompile("^[a-zA-Z0-9\\-]+$")
	)

	if name == "" {
		user, err := apiClient.GetCurrentUser()
		if err != nil {
			return nil, err
		}
		host, _ := os.Hostname()

		cleanEmailPattern := regexp.MustCompile("[^a-zA-Z0-9\\-]")
		name = fmt.Sprintf("interactive-%s-%s-%d",
			strings.Split(host, ".")[0],
			cleanEmailPattern.ReplaceAllString(user.Email, "-"), badrand.Intn(1000))
	}

	if regionCode == "" {
		region, err := apiClient.ClosestWireguardGatewayRegion()
		if err != nil {
			return nil, err
		}
		regionCode = region.Code
	}

	if !rx.MatchString(name) {
		return nil, errors.New("name must consist solely of letters, numbers, and the dash character")
	}

	fmt.Printf("Creating WireGuard peer \"%s\" in region \"%s\" for organization %s\n", name, regionCode, org.Slug)

	pubkey, privatekey := C25519pair()

	data, err := apiClient.CreateWireGuardPeer(org, regionCode, name, pubkey)
	if err != nil {
		return nil, err
	}

	return &wg.WireGuardState{
		Name:         name,
		Region:       regionCode,
		Org:          org.Slug,
		LocalPublic:  pubkey,
		LocalPrivate: privatekey,
		Peer:         *data,
	}, nil
}

func C25519pair() (string, string) {
	var private [32]byte
	_, err := rand.Read(private[:])
	if err != nil {
		panic(fmt.Sprintf("reading from random: %s", err))
	}

	public, err := curve25519.X25519(private[:], curve25519.Basepoint)
	if err != nil {
		panic(fmt.Sprintf("can't mult: %s", err))
	}

	return base64.StdEncoding.EncodeToString(public[:]),
		base64.StdEncoding.EncodeToString(private[:])
}

type WireGuardStates map[string]*wg.WireGuardState

func getWireGuardState() (WireGuardStates, error) {
	states := WireGuardStates{}

	if err := viper.UnmarshalKey(flyctl.ConfigWireGuardState, &states); err != nil {
		return nil, errors.Wrap(err, "invalid wireguard state")
	}

	return states, nil
}

func getWireGuardStateForOrg(orgSlug string) (*wg.WireGuardState, error) {
	states, err := getWireGuardState()
	if err != nil {
		return nil, err
	}

	return states[orgSlug], nil
}

func setWireGuardState(s WireGuardStates) error {
	viper.Set(flyctl.ConfigWireGuardState, s)
	if err := flyctl.SaveConfig(); err != nil {
		return errors.Wrap(err, "error saving config file")
	}

	return nil
}

func setWireGuardStateForOrg(orgSlug string, s *wg.WireGuardState) error {
	states, err := getWireGuardState()
	if err != nil {
		return err
	}

	states[orgSlug] = s

	return setWireGuardState(states)
}

func PruneInvalidPeers(apiClient *api.Client) error {
	state, err := getWireGuardState()
	if err != nil {
		return nil
	}

	peerIPs := []string{}

	for _, peer := range state {
		peerIPs = append(peerIPs, peer.Peer.Peerip)
	}

	invalidPeerIPs, err := apiClient.ValidateWireGuardPeers(peerIPs)
	if err != nil {
		return err
	}

	for _, invalidPeerIP := range invalidPeerIPs {
		for orgSlug, peer := range state {
			if peer.Peer.Peerip == invalidPeerIP {
				terminal.Debugf("removing invalid peer %s for organization %s", invalidPeerIP, orgSlug)
				delete(state, orgSlug)
			}
		}
	}

	return setWireGuardState(state)
}
