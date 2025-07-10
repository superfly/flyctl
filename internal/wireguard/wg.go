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
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/flyctl/wg"
	"golang.org/x/crypto/curve25519"
)

var cleanDNSPattern = regexp.MustCompile(`[^a-zA-Z0-9\\-]`)

type WebClient interface {
	ValidateWireGuardPeers(ctx context.Context, peerIPs []string) (invalid []string, err error)
}

func generatePeerName(ctx context.Context, apiClient flyutil.Client) (string, error) {
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

func StateForOrg(ctx context.Context, apiClient flyutil.Client, org *fly.Organization, regionCode string, name string, reestablish bool, network string) (*wg.WireGuardState, error) {
	state, err := getWireGuardStateForOrg(org.Slug, network)
	if err != nil {
		return nil, err
	}
	if state != nil && !reestablish && (regionCode == "" || state.Region == regionCode) {
		return state, nil
	}

	terminal.Debugf("Can't find matching WireGuard configuration; creating new one\n")

	stateb, err := Create(apiClient, org, regionCode, name, network, "interactive")
	if err != nil {
		return nil, err
	}

	if err := setWireGuardStateForOrg(ctx, org.Slug, network, stateb); err != nil {
		return nil, err
	}

	return stateb, nil
}

func Create(apiClient flyutil.Client, org *fly.Organization, regionCode, name, network string, namePrefix string) (*wg.WireGuardState, error) {
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

		name = fmt.Sprintf("%s-%s", namePrefix, n)
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

	data, err := apiClient.CreateWireGuardPeer(ctx, org, regionCode, name, pubkey, network)
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

func GetWireGuardState() (wg.States, error) {
	states := wg.States{}

	if err := viper.UnmarshalKey(flyctl.ConfigWireGuardState, &states); err != nil {
		return nil, errors.Wrap(err, "invalid wireguard state")
	}

	return states, nil
}

func getWireGuardStateForOrg(orgSlug string, network string) (*wg.WireGuardState, error) {
	states, err := GetWireGuardState()
	if err != nil {
		return nil, err
	}

	sk := orgSlug
	if network != "" {
		sk = fmt.Sprintf("%s-%s", orgSlug, network)
	}

	return states[sk], nil
}

func setWireGuardState(ctx context.Context, s wg.States) error {
	viper.Set(flyctl.ConfigWireGuardState, s)
	configPath := state.ConfigFile(ctx)
	if err := config.SetWireGuardState(configPath, s); err != nil {
		return errors.Wrap(err, "error saving config file")
	}

	return nil
}

func setWireGuardStateForOrg(ctx context.Context, orgSlug, network string, s *wg.WireGuardState) error {
	states, err := GetWireGuardState()
	if err != nil {
		return err
	}

	sk := orgSlug
	if network != "" {
		sk = fmt.Sprintf("%s-%s", orgSlug, network)
	}

	states[sk] = s

	return setWireGuardState(ctx, states)
}

func PruneInvalidPeers(ctx context.Context, apiClient WebClient) error {
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

	return setWireGuardState(ctx, state)
}
