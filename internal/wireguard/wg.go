package wireguard

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	badrand "math/rand"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/pkg/wg"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/crypto/curve25519"
)

func StateForOrg(apiClient *api.Client, org *api.Organization, regionCode string, name string) (*wg.WireGuardState, error) {
	var (
		svm map[string]interface{}
		ok  bool
	)

	sv := viper.Get(flyctl.ConfigWireGuardState)
	if sv != nil {
		terminal.Debugf("Found WireGuard state in local configuration\n")

		svm, ok = sv.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("garbage stored in wireguard_state in config")
		}

		// no state saved for this org
		savedStatev, ok := svm[org.Slug]
		if !ok {
			goto NEW_CONNECTION
		}

		savedPeerv, ok := savedStatev.(map[string]interface{})["peer"]
		if !ok {
			return nil, fmt.Errorf("garbage stored in wireguard_state in config (under peer)")
		}

		savedState := savedStatev.(map[string]interface{})
		savedPeer := savedPeerv.(map[string]interface{})

		// if we get this far and the config is garbled, i'm fine
		// with a panic
		return &wg.WireGuardState{
			Org:          org.Slug,
			Name:         savedState["name"].(string),
			Region:       savedState["region"].(string),
			LocalPublic:  savedState["localpublic"].(string),
			LocalPrivate: savedState["localprivate"].(string),
			Peer: api.CreatedWireGuardPeer{
				Peerip:     savedPeer["peerip"].(string),
				Endpointip: savedPeer["endpointip"].(string),
				Pubkey:     savedPeer["pubkey"].(string),
			},
		}, nil
	} else {
		svm = map[string]interface{}{}
	}

NEW_CONNECTION:
	terminal.Debugf("Can't find matching WireGuard configuration; creating new one\n")

	stateb, err := Create(apiClient, org, regionCode, name)
	if err != nil {
		return nil, err
	}

	svm[stateb.Org] = stateb

	viper.Set(flyctl.ConfigWireGuardState, &svm)
	if err := flyctl.SaveConfig(); err != nil {
		return nil, err
	}

	return stateb, err
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
