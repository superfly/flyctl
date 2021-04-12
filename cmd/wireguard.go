package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	badrand "math/rand"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/wg"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/crypto/curve25519"
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

func orgByArg(ctx *cmdctx.CmdContext) (*api.Organization, error) {
	client := ctx.Client.API()

	if len(ctx.Args) == 0 {
		org, err := selectOrganization(client, "")
		if err != nil {
			return nil, err
		}

		return org, nil
	}

	return client.FindOrganizationBySlug(ctx.Args[0])
}

func runWireGuardList(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	peers, err := client.GetWireGuardPeers(org.Slug)
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(peers)
		return nil
	}

	table := tablewriter.NewWriter(ctx.Out)

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

func c25519pair() (string, string) {
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

type WireGuardState struct {
	Org          string
	Name         string
	Region       string
	LocalPublic  string
	LocalPrivate string
	DNS          string
	Peer         api.CreatedWireGuardPeer
}

// BUG(tqbf): Obviously all this needs to go, and I should just
// make my code conform to the marshal/unmarshal protocol wireguard-go
// uses, but in the service of landing this feature, I'm just going
// to apply a layer of spackle for now.
func (s *WireGuardState) TunnelConfig() *wg.Config {
	skey := wg.PrivateKey{}
	if err := skey.UnmarshalText([]byte(s.LocalPrivate)); err != nil {
		panic(fmt.Sprintf("martian local private key: %s", err))
	}

	pkey := wg.PublicKey{}
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
	dns := net.ParseIP(fmt.Sprintf("%s", raddr))

	// BUG(tqbf): I think this dance just because these needed to
	// parse for Ben's TOML code.
	wgl := wg.IPNet(*lnet)
	wgr := wg.IPNet(*rnet)

	return &wg.Config{
		LocalPrivateKey: skey,
		LocalNetwork:    &wgl,
		RemotePublicKey: pkey,
		RemoteNetwork:   &wgr,
		Endpoint:        s.Peer.Endpointip + ":51820",
		DNS:             dns,
		// LogLevel:        9999999,
	}

}

func wireGuardCreate(ctx *cmdctx.CmdContext, org *api.Organization, regionp, namep *string) (*WireGuardState, error) {
	var (
		region, name string
		err          error
		rx           = regexp.MustCompile("^[a-zA-Z0-9\\-]+$")
		client       = ctx.Client.API()
	)

	if org == nil {
		org, err = orgByArg(ctx)
		if err != nil {
			return nil, err
		}
	}

	if regionp == nil {
		region, err = argOrPrompt(ctx, 1, "Region in which to add WireGuard peer: ")
		if err != nil {
			return nil, err
		}
	} else {
		region = *regionp
	}

	if namep == nil {
		for !rx.MatchString(name) {
			if name != "" {
				fmt.Println("Name must consist solely of letters, numbers, and the dash character.")
			}

			name, err = argOrPromptLoop(ctx, 2, "New DNS name for WireGuard peer: ", name)
			if err != nil {
				return nil, err
			}
		}
	} else {
		name = *namep
	}

	fmt.Printf("Creating WireGuard peer \"%s\" in region \"%s\" for organization %s\n", name, region, org.Slug)

	pubkey, privatekey := c25519pair()

	data, err := client.CreateWireGuardPeer(org, region, name, pubkey)
	if err != nil {
		return nil, err
	}

	return &WireGuardState{
		Name:         name,
		Region:       region,
		Org:          org.Slug,
		LocalPublic:  pubkey,
		LocalPrivate: privatekey,
		Peer:         *data,
	}, nil
}

func runWireGuardCreate(ctx *cmdctx.CmdContext) error {
	state, err := wireGuardCreate(ctx, nil, nil, nil)
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

func runWireGuardRemove(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	name, err := argOrPrompt(ctx, 1, "Name of WireGuard peer to remove: ")
	if err != nil {
		return err
	}

	fmt.Printf("Removing WireGuard peer \"%s\" for organization %s\n", name, org.Slug)

	err = client.RemoveWireGuardPeer(org, name)
	if err != nil {
		return err
	}

	fmt.Println("Removed peer.")

	return nil
}

func runWireGuardTokenList(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	tokens, err := client.GetDelegatedWireGuardTokens(org.Slug)
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(tokens)
		return nil
	}

	table := tablewriter.NewWriter(ctx.Out)

	table.SetHeader([]string{
		"Name",
	})

	for _, peer := range tokens {
		table.Append([]string{peer.Name})
	}

	table.Render()

	return nil
}

func runWireGuardTokenCreate(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	name, err := argOrPrompt(ctx, 1, "Memorable name for WireGuard token: ")
	if err != nil {
		return err
	}

	data, err := client.CreateDelegatedWireGuardToken(org, name)
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

	w, shouldClose, err := resolveOutputWriter(ctx, 2, "Filename to store WireGuard token in, or 'stdout': ")
	if err != nil {
		return err
	}
	if shouldClose {
		defer w.Close()
	}

	fmt.Fprintf(w, "FLY_WIREGUARD_TOKEN=%s\n", data.Token)

	return nil
}

func runWireGuardTokenDelete(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

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

	fmt.Printf("Removing WireGuard token \"%s\" for organization %s\n", kv, org.Slug)

	if tup[0] == "name" {
		err = client.DeleteDelegatedWireGuardToken(org, &tup[1], nil)
	} else {
		err = client.DeleteDelegatedWireGuardToken(org, nil, &tup[1])
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

	pubkey, privatekey := c25519pair()

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

	pubkey, privatekey := c25519pair()

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

func wireGuardForOrg(ctx *cmdctx.CmdContext, org *api.Organization) (*WireGuardState, error) {
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
		return &WireGuardState{
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

	user, err := ctx.Client.API().GetCurrentUser()
	if err != nil {
		return nil, err
	}

	rx := regexp.MustCompile("[^a-zA-Z0-9\\-]")

	host, _ := os.Hostname()

	wgName := fmt.Sprintf("interactive-%s-%s-%d",
		strings.Split(host, ".")[0],
		rx.ReplaceAllString(user.Email, "-"), badrand.Intn(1000))

	region := ctx.Config.GetString("region")

	terminal.Debugf("Creating new WireGuard connection for %s in %s named %s\n", org.Slug, region, wgName)

	stateb, err := wireGuardCreate(ctx, org, &region, &wgName)
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
