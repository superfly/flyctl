package cmd

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
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
	cmd.Hidden = true

	child := func(parent *Command, fn RunFn, ds string) *Command {
		return BuildCommandKS(parent, fn, docstrings.Get(ds), os.Stdout, requireSession)
	}

	child(cmd, runWireGuardList, "wireguard.list").Args = cobra.MaximumNArgs(1)
	child(cmd, runWireGuardCreate, "wireguard.create").Args = cobra.MaximumNArgs(4)
	child(cmd, runWireGuardRemove, "wireguard.remove").Args = cobra.MaximumNArgs(2)

	return cmd
}

func argOrPrompt(ctx *cmdctx.CmdContext, nth int, prompt string) (string, error) {
	if len(ctx.Args) >= (nth + 1) {
		return ctx.Args[nth], nil
	}

	val := ""
	err := survey.AskOne(&survey.Input{
		Message: prompt,
	}, &val)

	return val, err
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
Address = {{.Peer.Peerip}}/24
DNS = {{.Meta.DNS}}

[Peer]
PublicKey = {{.Peer.Pubkey}}
AllowedIPs = {{.Meta.AllowedIPs}}
Endpoint = {{.Peer.Endpointip}}:51820

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

	name, err := argOrPrompt(ctx, 2, "Name of WireGuard peer to add: ")
	if err != nil {
		return err
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

	var (
		w        io.Writer
		f        *os.File
		filename string
	)

	for w == nil {
		filename, err = argOrPrompt(ctx, 3, "Filename to store WireGuard configuration in, or 'stdout': ")
		if filename == "" {
			fmt.Println("Provide a filename (or 'stdout')")
			continue
		}

		if filename == "stdout" {
			w = os.Stdout
		} else {
			f, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				fmt.Printf("Can't create '%s': %s\n", filename, err)
				continue
			}

			w = f
			defer f.Close()
		}
	}

	generateWgConf(data, privatekey, w)

	if f != nil {
		fmt.Printf("Wrote WireGuard configuration to '%s'; load in your WireGuard client\n", filename)
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
