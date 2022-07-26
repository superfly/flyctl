package wireguard

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/oklog/ulid/v2"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/flyctl/wg"
	"golang.org/x/crypto/curve25519"
)

var (
	cleanDNSPattern = regexp.MustCompile(`[^a-zA-Z0-9\\-]`)
)

func generatePeerName(ctx context.Context, apiClient *api.Client) (string, error) {
	user, err := apiClient.GetCurrentUser(ctx)
	if err != nil {
		return "", err
	}
	emailSlug := cleanDNSPattern.ReplaceAllString(user.Email, "-")

	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	hostSlug := cleanDNSPattern.ReplaceAllString(strings.Split(host, ".")[0], "-")

	name := fmt.Sprintf("%s-%s-%s", hostSlug, emailSlug, ulid.Make())
	return name, nil
}

func StateForOrg(apiClient *api.Client, org *api.Organization, regionCode string, name string, recycle bool) (*wg.WireGuardState, error) {
	state, err := getWireGuardStateForOrg(org.Slug)
	if err != nil {
		return nil, err
	}
	if state != nil && !recycle {
		return state, nil
	}

	terminal.Debugf("Can't find matching WireGuard configuration; creating new one\n")

	ctx := context.TODO()
	if name == "" {
		n, err := generatePeerName(ctx, apiClient)
		if err != nil {
			return nil, err
		}

		name = fmt.Sprintf("interactive-agent-%s", n)
	}

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
	ctx := context.TODO()
	var (
		err error
		rx  = regexp.MustCompile(`^[a-zA-Z0-9\\-]+$`)
	)

	if name == "" {
		n, err := generatePeerName(ctx, apiClient)
		if err != nil {
			return nil, err
		}

		name = fmt.Sprintf("interactive-%s", n)
	}

	if regionCode == "" {
		regionCode = os.Getenv("FLYCTL_WG_REGION")
	}

	if regionCode == "" {
		region, err := apiClient.ClosestWireguardGatewayRegion(ctx)
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

	data, err := apiClient.CreateWireGuardPeer(ctx, org, regionCode, name, pubkey)
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

	return base64.StdEncoding.EncodeToString(public),
		base64.StdEncoding.EncodeToString(private[:])
}

type WireGuardStates map[string]*wg.WireGuardState

func GetWireGuardState() (WireGuardStates, error) {
	states := WireGuardStates{}

	if err := viper.UnmarshalKey(flyctl.ConfigWireGuardState, &states); err != nil {
		return nil, errors.Wrap(err, "invalid wireguard state")
	}

	return states, nil
}

func getWireGuardStateForOrg(orgSlug string) (*wg.WireGuardState, error) {
	states, err := GetWireGuardState()
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
	states, err := GetWireGuardState()
	if err != nil {
		return err
	}

	states[orgSlug] = s

	return setWireGuardState(states)
}

func PruneInvalidPeers(ctx context.Context, apiClient *api.Client) error {
	state, err := GetWireGuardState()
	if err != nil {
		return nil
	}

	peerIPs := make([]string, 0, len(state))
	for _, peer := range state {
		peerIPs = append(peerIPs, peer.Peer.Peerip)
	}

	invalidPeerIPs, err := apiClient.ValidateWireGuardPeers(ctx, peerIPs)
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
