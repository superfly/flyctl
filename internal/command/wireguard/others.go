package wireguard

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func argOrPrompt(ctx context.Context, nth int, prompt string) (string, error) {
	args := flag.Args(ctx)
	if len(args) >= (nth + 1) {
		return args[nth], nil
	}

	val := ""
	err := survey.AskOne(
		&survey.Input{Message: prompt},
		&val,
	)

	return val, err
}

func orgByArg(ctx context.Context) (*fly.Organization, error) {
	args := flag.Args(ctx)

	if len(args) == 0 {
		org, err := prompt.Org(ctx)
		if err != nil {
			return nil, err
		}

		return org, nil
	}

	apiClient := flyutil.ClientFromContext(ctx)
	return apiClient.GetOrganizationBySlug(ctx, args[0])
}

func resolveOutputWriter(ctx context.Context, idx int, prompt string) (w io.WriteCloser, mustClose bool, err error) {
	io := iostreams.FromContext(ctx)
	var f *os.File
	var filename string

	for {
		filename, err = argOrPrompt(ctx, idx, prompt)
		if err != nil {
			return nil, false, err
		}

		if filename == "" {
			fmt.Fprintln(io.Out, "Provide a filename (or 'stdout')")
			continue
		}

		if filename == "stdout" {
			return os.Stdout, false, nil
		}

		f, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return f, true, nil
		}

		fmt.Fprintf(io.Out, "Can't create '%s': %s\n", filename, err)
	}
}

func generateWgConf(peer *fly.CreatedWireGuardPeer, privkey string, w io.Writer) {
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
		Peer *fly.CreatedWireGuardPeer
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

	data.Meta.DNS = addr.String()
	data.Meta.Privkey = privkey

	tmpl := template.Must(template.New("name").Parse(templateStr))
	tmpl.Execute(w, &data)
}

func selectWireGuardPeer(ctx context.Context, client flyutil.Client, slug string) (string, error) {
	peers, err := client.GetWireGuardPeers(ctx, slug)
	if err != nil {
		return "", err
	}

	if len(peers) < 1 {
		return "", fmt.Errorf(`Organization "%s" does not have any wireguard peers`, slug)
	}

	var options []string
	for _, peer := range peers {
		options = append(options, peer.Name)
	}

	selectedPeer := 0
	prompt := &survey.Select{
		Message:  "Select peer:",
		Options:  options,
		PageSize: 30,
	}
	if err := survey.AskOne(prompt, &selectedPeer); err != nil {
		return "", err
	}

	return peers[selectedPeer].Name, nil
}
