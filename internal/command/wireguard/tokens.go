package wireguard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/iostreams"
)

func runWireguardTokenList(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	tokens, err := apiClient.GetDelegatedWireGuardTokens(ctx, org.Slug)
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, tokens)
		return nil
	}

	table := tablewriter.NewWriter(io.Out)
	table.SetHeader([]string{"Name"})
	for _, peer := range tokens {
		table.Append([]string{peer.Name})
	}
	table.Render()

	return nil
}

func runWireguardTokenCreate(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	name, err := argOrPrompt(ctx, 1, "Memorable name for WireGuard token: ")
	if err != nil {
		return err
	}

	data, err := apiClient.CreateDelegatedWireGuardToken(ctx, org, name)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, `
!!!! WARNING: Output includes credential information. Credentials cannot !!!!
!!!! be recovered after creation; if you lose the token, you'll need to  !!!!
!!!! remove and and re-add it.																		 			 !!!!

To use a token to create a WireGuard connection, you can use curl:

    curl -v --request POST
         -H "Authorization: Bearer ${WG_TOKEN}"
         -H "Content-Type: application/json"
         --data '{"name": "node-1", \
                  "group": "k8s",   \
                  "pubkey": "'"${WG_PUBKEY}"'", \
                  "region": "dev"}'
         http://fly.io/api/v3/wire_guard_peers

We'll return 'us' (our local 6PN address), 'them' (the gateway IP address),
and 'pubkey' (the public key of the gateway), which you can inject into a
"wg.con".
`)

	w, shouldClose, err := resolveOutputWriter(ctx, 2, "Filename to store WireGuard token in, or 'stdout': ")
	if err != nil {
		return err
	}
	if shouldClose {
		defer w.Close() // skipcq: GO-S2307
	}

	fmt.Fprintf(w, "FLY_WIREGUARD_TOKEN=%s\n", data.Token)

	return nil
}

func runWireguardTokenDelete(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	kv, err := argOrPrompt(ctx, 1, "'name:<name>' or token:<token>': ")
	if err != nil {
		return err
	}

	tup := strings.SplitN(kv, ":", 2)
	if len(tup) != 2 || (tup[0] != "name" && tup[0] != "token") {
		return fmt.Errorf("format is name:<name> or token:<token>")
	}

	fmt.Fprintf(io.Out, "Removing WireGuard token \"%s\" for organization %s\n", kv, org.Slug)

	if tup[0] == "name" {
		err = apiClient.DeleteDelegatedWireGuardToken(ctx, org, &tup[1], nil)
	} else {
		err = apiClient.DeleteDelegatedWireGuardToken(ctx, org, nil, &tup[1])
	}
	if err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Removed token.")
	return nil
}

func tokenRequest(method, path, token string, data interface{}) (*http.Response, error) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(data); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method,
		fmt.Sprintf("https://fly.io/api/v3/wire_guard_peers%s", path),
		buf)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fly.AuthorizationHeader(token))
	req.Header.Add("Content-Type", "application/json")

	return (&http.Client{}).Do(req)
}

type StartPeerJson struct {
	Name   string `json:"name"`
	Group  string `json:"group"`
	Pubkey string `json:"pubkey"`
	Region string `json:"region"`
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

func generateTokenConf(ctx context.Context, idx int, stat *PeerStatusJson, privkey string) error {
	fmt.Printf(`
!!!! WARNING: Output includes private key. Private keys cannot be recovered !!!!
!!!! after creating the peer; if you lose the key, you'll need to rekey     !!!!
!!!! the peering connection.                                                !!!!
`)

	w, shouldClose, err := resolveOutputWriter(ctx, idx, "Filename to store WireGuard configuration in, or 'stdout': ")
	if err != nil {
		return err
	}
	if shouldClose {
		defer w.Close() // skipcq: GO-S2307
	}

	generateWgConf(&fly.CreatedWireGuardPeer{
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

func runWireguardTokenStart(ctx context.Context) error {
	token := os.Getenv("FLY_WIREGUARD_TOKEN")
	if token == "" {
		return fmt.Errorf("set FLY_WIREGUARD_TOKEN env")
	}

	name, err := argOrPrompt(ctx, 0, "Name (DNS-compatible) for peer: ")
	if err != nil {
		return err
	}

	group, err := argOrPrompt(ctx, 1, "Peer group (i.e. 'k8s'): ")
	if err != nil {
		return err
	}

	region, err := argOrPrompt(ctx, 2, "Gateway region: ")
	if err != nil {
		return err
	}

	pubkey, privatekey := wireguard.C25519pair()

	body := &StartPeerJson{
		Name:   name,
		Group:  group,
		Pubkey: pubkey,
		Region: region,
	}

	resp, err := tokenRequest("POST", "", token, body)
	if err != nil {
		return err
	}

	peerStatus := &PeerStatusJson{}
	if err = json.NewDecoder(resp.Body).Decode(peerStatus); err != nil {
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

func runWireguardTokenUpdate(ctx context.Context) error {
	token := os.Getenv("FLY_WIREGUARD_TOKEN")
	if token == "" {
		return fmt.Errorf("set FLY_WIREGUARD_TOKEN env")
	}

	name, err := argOrPrompt(ctx, 0, "Name (DNS-compatible) for peer: ")
	if err != nil {
		return err
	}

	pubkey, privatekey := wireguard.C25519pair()

	body := &StartPeerJson{
		Pubkey: pubkey,
	}

	resp, err := tokenRequest("PUT", "/"+name, token, body)
	if err != nil {
		return err
	}

	peerStatus := &PeerStatusJson{}
	if err = json.NewDecoder(resp.Body).Decode(peerStatus); err != nil {
		if resp.StatusCode != 200 {
			return fmt.Errorf("server returned error: %s %w", resp.Status, err)
		}

		return err
	}

	if peerStatus.Error != "" {
		return fmt.Errorf("WireGuard API error: %s", peerStatus.Error)
	}

	if err = generateTokenConf(ctx, 1, peerStatus, privatekey); err != nil {
		return err
	}

	return nil
}
