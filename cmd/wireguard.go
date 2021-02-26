package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"golang.org/x/crypto/curve25519"
)

func newWireGuardCommand() *Command {
	cmd := BuildCommandKS(nil, nil, docstrings.Get("wireguard"), os.Stdout, requireSession)
	cmd.Aliases = []string{"wg"}

	child := func(parent *Command, fn RunFn, ds string) *Command {
		return BuildCommandKS(parent, fn, docstrings.Get(ds), os.Stdout, requireSession)
	}

	child(cmd, runWireGuardList, "wireguard.list").Args = cobra.MaximumNArgs(1)
	child(cmd, runWireGuardCreate, "wireguard.create").Args = cobra.MaximumNArgs(4)
	child(cmd, runWireGuardRemove, "wireguard.remove").Args = cobra.MaximumNArgs(2)

	tokens := child(cmd, nil, "wireguard.token")

	child(tokens, runWireGuardTokenList, "wireguard.token.list").Args = cobra.MaximumNArgs(1)
	child(tokens, runWireGuardTokenCreate, "wireguard.token.create").Args = cobra.MaximumNArgs(2)
	child(tokens, runWireGuardTokenDelete, "wireguard.token.delete").Args = cobra.MaximumNArgs(3)

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

func runWireGuardCreate(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	region, err := argOrPrompt(ctx, 1, "Region in which to add WireGuard peer: ")
	if err != nil {
		return err
	}

	var name string
	rx := regexp.MustCompile("^[a-zA-Z0-9\\-]+$")

	for !rx.MatchString(name) {
		if name != "" {
			fmt.Println("Name must consist solely of letters, numbers, and the dash character.")
		}

		name, err = argOrPromptLoop(ctx, 2, "New DNS name for WireGuard peer: ", name)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Creating WireGuard peer \"%s\" in region \"%s\" for organization %s\n", name, region, org.Slug)

	pubkey, privatekey := c25519pair()

	data, err := client.CreateWireGuardPeer(org, region, name, pubkey)
	if err != nil {
		return err
	}

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

	generateWgConf(data, privatekey, w)

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

	fmt.Fprintf(w, "WIREGUARD_TOKEN=%s\n", data.Token)

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
