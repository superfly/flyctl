package token

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/wireguard"
)

func newStart() (cmd *cobra.Command) {
	const (
		short = "Start a WireGuard peer connection off a token"
		long  = short + "\n"
		usage = "start [-name NAME] [-group GROUP] [-region REGION] [-token TOKEN] [FILE]"
	)

	cmd = command.New(usage, short, long, runStart,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.String{
			Name: "",
		})

	cmd.Args = cobra.ExactArgs(1)

	return
}

func runStart(ctx context.Context) error {
	token := os.Getenv("FLY_WIREGUARD_TOKEN")
	if token == "" {
		return fmt.Errorf("set FLY_WIREGUARD_TOKEN env")
	}

	const namePrompt = "Name (DNS-compatible) for peer: "
	name, err := prompt.UnlessStringFlag(ctx, "name", namePrompt, "", true)
	if err != nil {
		return err
	}

	const groupPrompt = "Peer group (i.e. 'k8s'): "
	group, err := prompt.UnlessStringFlag(ctx, "group", groupPrompt, "", true)
	if err != nil {
		return err
	}

	const regionPrompt = "Gateway region: "
	region, err := prompt.UnlessStringFlag(ctx, "region", regionPrompt, "", true)
	if err != nil {
		return err
	}

	pubkey, privatekey := wireguard.C25519pair()

	body := struct {
		Name   string `json:"name"`
		Group  string `json:"group"`
		Pubkey string `json:"pubkey"`
		Region string `json:"region"`
	}{
		Name:   name,
		Group:  group,
		Pubkey: pubkey,
		Region: region,
	}

	res, err := request(ctx, http.MethodPost, "", token, body)
	if err != nil {
		return fmt.Errorf("failed starting peer: %w", err)
	}
	defer res.Body.Close()

	peerStatus := &PeerStatusJson{}
	if err := json.NewDecoder(resp.Body).Decode(peerStatus); err != nil {
		if resp.StatusCode != 200 {
			return fmt.Errorf("server returned error: %s %w", resp.Status, err)
		}

		return err
	}

	if peerStatus.Error != "" {
		return fmt.Errorf("WireGuard API error: %s", peerStatus.Error)
	}

	if err = generateTokenConf(ctx, 3, peerStatus, privatekey); err != nil {
		return err
	}

	return nil
}

type startPeer struct {
}

type UpdatePeerJson struct {
	Pubkey string `json:"pubkey"`
}

type PeerStatusJson struct {
	Us     string `json:"us"`
	Them   string `json:"them"`
	Pubkey string `json:"key"`
	Error  string `json:"error"`
}
