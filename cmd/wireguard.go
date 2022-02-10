package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/wireguard"
)

func newWireGuardCommand(client *client.Client) *Command {
	cmd := BuildCommandKS(nil, nil, docstrings.Get("wireguard"), client, requireSession)
	cmd.Aliases = []string{"wg"}

	child := func(parent *Command, fn RunFn, ds string) *Command {
		return BuildCommandKS(parent, fn, docstrings.Get(ds), client, requireSession)
	}

	child(cmd, runWireGuardList, "wireguard.list").Args = cobra.MaximumNArgs(1)
	child(cmd, runWireGuardCreate, "wireguard.create").Args = cobra.MaximumNArgs(4)
	child(cmd, runWireGuardRemove, "wireguard.remove").Args = cobra.MaximumNArgs(2)
	child(cmd, runWireGuardStat, "wireguard.status").Args = cobra.MaximumNArgs(2)

	tokens := child(cmd, nil, "wireguard.token")

	child(tokens, runWireGuardTokenList, "wireguard.token.list").Args = cobra.MaximumNArgs(1)
	child(tokens, runWireGuardTokenCreate, "wireguard.token.create").Args = cobra.MaximumNArgs(2)
	child(tokens, runWireGuardTokenDelete, "wireguard.token.delete").Args = cobra.MaximumNArgs(3)

	child(tokens, runWireGuardTokenStartPeer, "wireguard.token.start").Args = cobra.MaximumNArgs(4)
	child(tokens, runWireGuardTokenUpdatePeer, "wireguard.token.update").Args = cobra.MaximumNArgs(2)

	return cmd
}

func argOrPromptImpl(ctx *cmdctx.CmdContext, nth int, prompt string, first bool) (string, error) {
	if len(ctx.Args) >= (nth + 1) {
		return ctx.Args[nth], nil
	}

	val := ""
	err := survey.AskOne(&survey.Input{
		Message: prompt,
	}, &val)

	return val, err
}

func argOrPromptLoop(ctx *cmdctx.CmdContext, nth int, prompt, last string) (string, error) {
	return argOrPromptImpl(ctx, nth, prompt, last == "")
}

func argOrPrompt(ctx *cmdctx.CmdContext, nth int, prompt string) (string, error) {
	return argOrPromptImpl(ctx, nth, prompt, true)
}

func orgByArg(cmdCtx *cmdctx.CmdContext) (*api.Organization, error) {
	ctx := cmdCtx.Command.Context()
	client := cmdCtx.Client.API()

	if len(cmdCtx.Args) == 0 {
		org, err := selectOrganization(ctx, client, "", nil)
		if err != nil {
			return nil, err
		}

		return org, nil
	}

	return client.FindOrganizationBySlug(ctx, cmdCtx.Args[0])
}

func runWireGuardList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	client := cmdCtx.Client.API()

	org, err := orgByArg(cmdCtx)
	if err != nil {
		return err
	}

	peers, err := client.GetWireGuardPeers(ctx, org.Slug)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(peers)
		return nil
	}

	table := tablewriter.NewWriter(cmdCtx.Out)

	table.SetHeader([]string{
		"Name",
		"Region",
		"Peer IP",
	})

	for _, peer := range peers {
		table.Append([]string{peer.Name, peer.Region, peer.Peerip})
	}

	table.Render()

	return nil
}

func generateWgConf(peer *api.CreatedWireGuardPeer, privkey string, w io.Writer) {
	templateStr := `
[Interface]
PrivateKey = {{.Meta.Privkey}}
Address = {{.Peer.Peerip}}/120
DNS = {{.Meta.DNS}}

[Peer]
PublicKey = {{.Peer.Pubkey}}
AllowedIPs = {{.Meta.AllowedIPs}}
Endpoint = {{.Peer.Endpointip}}:51820
PersistentKeepalive = 15

`
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
	for i := 6; i < 16; i++ {
		addr[i] = 0
	}

	// BUG(tqbf): can't stay this way
	data.Meta.AllowedIPs = fmt.Sprintf("%s/48", addr)

	addr[15] = 3

	data.Meta.DNS = fmt.Sprintf("%s", addr)
	data.Meta.Privkey = privkey

	tmpl := template.Must(template.New("name").Parse(templateStr))

	tmpl.Execute(w, &data)
}

func resolveOutputWriter(ctx *cmdctx.CmdContext, idx int, prompt string) (w io.WriteCloser, mustClose bool, err error) {
	var (
		f        *os.File
		filename string
	)

	for {
		filename, err = argOrPromptLoop(ctx, idx, prompt, filename)
		if err != nil {
			return nil, false, err
		}

		if filename == "" {
			fmt.Println("Provide a filename (or 'stdout')")
			continue
		}

		if filename == "stdout" {
			return os.Stdout, false, nil
		}

		f, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			return f, true, nil
		}

		fmt.Printf("Can't create '%s': %s\n", filename, err)
	}
}

func runWireGuardCreate(ctx *cmdctx.CmdContext) error {

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	var region string
	var name string

	if len(ctx.Args) > 1 && ctx.Args[1] != "" {
		region = ctx.Args[1]
	}

	if len(ctx.Args) > 2 && ctx.Args[2] != "" {
		name = ctx.Args[2]
	}

	state, err := wireguard.Create(ctx.Client.API(), org, region, name)
	if err != nil {
		return err
	}

	data := &state.Peer

	fmt.Printf(`
!!!! WARNING: Output includes private key. Private keys cannot be recovered !!!!
!!!! after creating the peer; if you lose the key, you'll need to remove    !!!!
!!!! and re-add the peering connection.                                     !!!!
`)

	w, shouldClose, err := resolveOutputWriter(ctx, 3, "Filename to store WireGuard configuration in, or 'stdout': ")
	if err != nil {
		return err
	}
	if shouldClose {
		defer w.Close()
	}

	generateWgConf(data, state.LocalPrivate, w)

	if shouldClose {
		filename := w.(*os.File).Name()
		fmt.Printf("Wrote WireGuard configuration to %s; load in your WireGuard client\n", filename)
	}

	return nil
}

func runWireGuardRemove(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	client := cmdCtx.Client.API()

	org, err := orgByArg(cmdCtx)
	if err != nil {
		return err
	}

	var name string
	if len(cmdCtx.Args) >= 2 {
		name = cmdCtx.Args[1]
	} else {
		name, err = selectWireGuardPeer(ctx, cmdCtx.Client.API(), org.Slug)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Removing WireGuard peer \"%s\" for organization %s\n", name, org.Slug)

	err = client.RemoveWireGuardPeer(ctx, org, name)
	if err != nil {
		return err
	}

	fmt.Println("Removed peer.")

	return wireguard.PruneInvalidPeers(ctx, cmdCtx.Client.API())
}

func runWireGuardStat(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	client := cmdCtx.Client.API()

	org, err := orgByArg(cmdCtx)
	if err != nil {
		return err
	}

	var name string
	if len(cmdCtx.Args) >= 2 {
		name = cmdCtx.Args[1]
	} else {
		name, err = selectWireGuardPeer(ctx, cmdCtx.Client.API(), org.Slug)
		if err != nil {
			return err
		}
	}

	status, err := client.GetWireGuardPeerStatus(ctx, org.Slug, name)
	if err != nil {
		return err
	}

	fmt.Printf("Alive: %+v\n", status.Live)

	if status.WgError != "" {
		fmt.Printf("Gateway error: %s\n", status.WgError)
	}

	if !status.Live {
		return nil
	}

	if status.Endpoint != "" {
		fmt.Printf("Last Source Address: %s\n", status.Endpoint)
	}

	ago := ""
	if status.SinceAdded != "" {
		ago = " (" + status.SinceAdded + " ago)"
	}

	if status.LastHandshake != "" {
		fmt.Printf("Last Handshake At: %s%s\n", status.LastHandshake, ago)
	}

	ago = ""
	if status.SinceHandshake != "" {
		ago = " (" + status.SinceHandshake + " ago)"
	}

	fmt.Printf("Installed On Gateway At: %s%s\n", status.Added, ago)

	fmt.Printf("Traffic: rx:%d tx:%d\n", status.Rx, status.Tx)

	return nil
}

func runWireGuardTokenList(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	client := cmdCtx.Client.API()

	org, err := orgByArg(cmdCtx)
	if err != nil {
		return err
	}

	tokens, err := client.GetDelegatedWireGuardTokens(ctx, org.Slug)
	if err != nil {
		return err
	}

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(tokens)
		return nil
	}

	table := tablewriter.NewWriter(cmdCtx.Out)

	table.SetHeader([]string{
		"Name",
	})

	for _, peer := range tokens {
		table.Append([]string{peer.Name})
	}

	table.Render()

	return nil
}

func runWireGuardTokenCreate(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	client := cmdCtx.Client.API()

	org, err := orgByArg(cmdCtx)
	if err != nil {
		return err
	}

	name, err := argOrPrompt(cmdCtx, 1, "Memorable name for WireGuard token: ")
	if err != nil {
		return err
	}

	data, err := client.CreateDelegatedWireGuardToken(ctx, org, name)
	if err != nil {
		return err
	}

	fmt.Printf(`
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

	w, shouldClose, err := resolveOutputWriter(cmdCtx, 2, "Filename to store WireGuard token in, or 'stdout': ")
	if err != nil {
		return err
	}
	if shouldClose {
		defer w.Close()
	}

	fmt.Fprintf(w, "FLY_WIREGUARD_TOKEN=%s\n", data.Token)

	return nil
}

func runWireGuardTokenDelete(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	client := cmdCtx.Client.API()

	org, err := orgByArg(cmdCtx)
	if err != nil {
		return err
	}

	kv, err := argOrPrompt(cmdCtx, 1, "'name:<name>' or token:<token>': ")
	if err != nil {
		return err
	}

	tup := strings.SplitN(kv, ":", 2)
	if len(tup) != 2 || (tup[0] != "name" && tup[0] != "token") {
		return fmt.Errorf("format is name:<name> or token:<token>")
	}

	fmt.Printf("Removing WireGuard token \"%s\" for organization %s\n", kv, org.Slug)

	if tup[0] == "name" {
		err = client.DeleteDelegatedWireGuardToken(ctx, org, &tup[1], nil)
	} else {
		err = client.DeleteDelegatedWireGuardToken(ctx, org, nil, &tup[1])
	}
	if err != nil {
		return err
	}

	fmt.Println("Removed token.")

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
	req.Header.Add("Authorization", "Bearer "+token)
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

func generateTokenConf(ctx *cmdctx.CmdContext, idx int, stat *PeerStatusJson, privkey string) error {
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

func runWireGuardTokenStartPeer(ctx *cmdctx.CmdContext) error {
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

func runWireGuardTokenUpdatePeer(ctx *cmdctx.CmdContext) error {
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
