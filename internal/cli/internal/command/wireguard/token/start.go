package token

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/prompt"
	"github.com/superfly/flyctl/internal/wireguard"
)

func newStart() (cmd *cobra.Command) {
	const (
		short = "Start a WireGuard peer connection off a token"
		long  = short + "\n"
		usage = "start [-name NAME] [-group GROUP] [-region REGION] [FILE]"
	)

	cmd = command.New(usage, short, long, runStart,
		command.RequireSession,
	)

	flag.Add(cmd,
		flag.String{
			Name:        "name",
			Description: "A DNS-compatible name for the peer",
		},
		flag.String{
			Name:        "group",
			Description: "The peer's group (i.e. 'k8s')",
		},
		flag.Region(),
	)

	cmd.Args = cobra.MaximumNArgs(1)

	return
}

func runStart(ctx context.Context) error {
	token := os.Getenv("FLY_WIREGUARD_TOKEN")
	if token == "" {
		return errors.New("set FLY_WIREGUARD_TOKEN env")
	}

	const namePrompt = "Name (DNS-compatible) for peer:"
	name, err := prompt.UnlessStringFlag(ctx, "name", namePrompt, "", true)
	if err != nil {
		return err
	}

	const groupPrompt = "Peer group (i.e. 'k8s'):"
	group, err := prompt.UnlessStringFlag(ctx, "group", groupPrompt, "", true)
	if err != nil {
		return err
	}

	region, err := prompt.Region(ctx)
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
		Region: region.Code,
	}

	res, err := request(ctx, http.MethodPost, "", token, body)
	if err != nil {
		return fmt.Errorf("failed starting peer: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error code (%d): %w", res.StatusCode, err)
	}

	var status peerStatus
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		return fmt.Errorf("failed unmarshaling server response: %w", err)
	}

	if status.Error != "" {
		return fmt.Errorf("WireGuard API error: %w", status.Error)
	}

	var buf bytes.Buffer
	if err := generateConfig(&buf); err != nil {
		return fmt.Errorf("failed generating config: %w", err)
	}

	io := iostreams.FromContext(ctx)

	path := flag.FirstArg(ctx)
	if path == "" {
		writeConfig(io.Out, status)
	}

	if err = generateTokenConf(ctx, 3, peerStatus, privatekey); err != nil {
		return err
	}

	return nil
}

type peerStatus struct {
	Us     string `json:"us"`
	Them   string `json:"them"`
	Pubkey string `json:"key"`
	Error  string `json:"error"`
}

func (ps *peerStatus) generateTokenConf(ctx context.Context) {
	stderr := iostreams.FromContext(ctx).ErrOut

	fmt.Fprintf(stderr, `
!!!! WARNING: Output includes private key. Private keys cannot be recovered !!!!
!!!! after creating the peer; if you lose the key, you'll need to rekey     !!!!
!!!! the peering connection.                                                !!!!
`)

	w, shouldClose, err := resolveOutputWriter(ctx, idx, "Filename to store WireGuard configuration in, or 'stdout': ")
	if err != nil {
		return err
	}
	if shouldClose {
		defer w.Close()
	}

	generateWgConf(&api.CreatedWireGuardPeer{
		Peerip:     stat.Us,
		Pubkey:     stat.Pubkey,
		Endpointip: stat.Them,
	}, privkey, w)

	if shouldClose {
		filename := w.(*os.File).Name()
		fmt.Printf("Wrote WireGuard configuration to %s; load in your WireGuard client\n", filename)
	}

	return nil
}

var (
	//go:embed template.gotext
	confTemplateBody string

	confTemplate = template.Must(template.New("conf").Parse(confTemplateBody))
)

func generateConfig(w io.Writer, peer *api.CreatedWireGuardPeer, privkey string) error {
	data := struct {
		Peer *api.CreatedWireGuardPeer
		Meta struct {
			Privkey    string
			AllowedIPs string
			DNS        string
		}
	}{
		Peer: peer,
	}

	addr := net.ParseIP(peer.Peerip).To16()
	for i := 6; i < 15; i++ {
		addr[i] = 0
	}
	addr[15] = 3

	// BUG(tqbf): can't stay this way
	data.Meta.AllowedIPs = fmt.Sprintf("%s/48", addr)

	data.Meta.DNS = fmt.Sprintf("%s", addr)
	data.Meta.Privkey = privkey

	return confTemplate.Execute(w, &data)
}
