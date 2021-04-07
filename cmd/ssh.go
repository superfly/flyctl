package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/mail"
	"os"
	"strings"

	"github.com/ejcx/sshcert"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func newSSHCommand(client *client.Client) *Command {
	cmd := BuildCommandKS(nil, nil, docstrings.Get("ssh"), client, requireSession)

	child := func(parent *Command, fn RunFn, ds string) *Command {
		return BuildCommandKS(parent, fn, docstrings.Get(ds), client, requireSession)
	}

	child(cmd, runSSHLog, "ssh.log").Args = cobra.MaximumNArgs(1)
	child(cmd, runSSHEstablish, "ssh.establish").Args = cobra.MaximumNArgs(2)

	console := BuildCommandKS(cmd,
		runSSHConsole,
		docstrings.Get("ssh.console"),
		client,
		requireSession,
		requireAppName)
	console.Args = cobra.MaximumNArgs(1)

	console.AddBoolFlag(BoolFlagOpts{
		Name:        "select",
		Shorthand:   "s",
		Default:     false,
		Description: "select available instances",
	})

	console.AddBoolFlag(BoolFlagOpts{
		Name:        "probe",
		Shorthand:   "p",
		Default:     false,
		Description: "test WireGuard connection after establishing",
	})

	console.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Shorthand:   "r",
		Description: "Region to create WireGuard connection in",
	})

	issue := child(cmd, runSSHIssue, "ssh.issue")
	issue.Args = cobra.MaximumNArgs(3)

	issue.AddStringFlag(StringFlagOpts{
		Name:        "username",
		Shorthand:   "u",
		Description: "Unix username for SSH cert",
	})

	issue.AddIntFlag(IntFlagOpts{
		Name:        "hours",
		Default:     24,
		Description: "Expiration, in hours (<72)",
	})

	issue.AddBoolFlag(BoolFlagOpts{
		Name:        "agent",
		Shorthand:   "a",
		Default:     false,
		Description: "Add key to SSH agent",
	})

	issue.AddBoolFlag(BoolFlagOpts{
		Name:        "dotssh",
		Shorthand:   "d",
		Default:     false,
		Description: "Store keys in ~/.ssh, like normal keys",
	})

	issue.AddBoolFlag(BoolFlagOpts{
		Name:        "overwrite",
		Shorthand:   "o",
		Default:     false,
		Description: "Overwrite existing SSH keys in same location, if we generated them",
	})

	shell := child(cmd, runSSHShell, "ssh.shell")
	shell.Args = cobra.MaximumNArgs(2)

	shell.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Shorthand:   "r",
		Description: "Region to create WireGuard connection in",
	})

	return cmd
}

func runSSHLog(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	certs, err := client.GetLoggedCertificates(org.Slug)
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(certs)
		return nil
	}

	table := tablewriter.NewWriter(ctx.Out)

	table.SetHeader([]string{
		"Root",
		"Certificate",
	})

	for _, cert := range certs {
		root := "no"
		if cert.Root {
			root = "yes"
		}

		first := true
		buf := &bytes.Buffer{}
		for i, ch := range cert.Cert {
			buf.WriteRune(ch)
			if i%60 == 0 && i != 0 {
				table.Append([]string{root, buf.String()})
				if first {
					root = ""
					first = false
				}
				buf.Reset()
			}
		}

		if buf.Len() != 0 {
			table.Append([]string{root, buf.String()})
		}
	}

	table.Render()

	return nil
}

func runSSHEstablish(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	override := false
	if len(ctx.Args) >= 1 && ctx.Args[1] == "override" {
		override = true
	}

	fmt.Printf("Establishing SSH CA cert for organization %s\n", org.Slug)

	cert, err := client.EstablishSSHKey(org, override)
	if err != nil {
		return err
	}

	fmt.Printf("New organization root certificate:\n%s", cert.Certificate)

	return nil
}

func singleUseSSHCertificate(ctx *cmdctx.CmdContext, org *api.Organization) (*api.IssuedCertificate, error) {
	client := ctx.Client.API()

	user, err := ctx.Client.API().GetCurrentUser()
	if err != nil {
		return nil, err
	}

	hours := 1
	return client.IssueSSHCertificate(org, user.Email, nil, &hours)
}

func runSSHIssue(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	var (
		emails string
		email  *mail.Address
	)

	for email == nil {
		prompt := "Email address for user to issue cert: "
		emails, err = argOrPromptLoop(ctx, 1, prompt, emails)
		if err != nil {
			return err
		}

		email, err = mail.ParseAddress(emails)
		if err != nil {
			ctx.Statusf("ssh", cmdctx.SERROR, "Invalid email address: %s (keep it simple!)\n", err)
			email = nil
		}
	}

	var (
		username *string
	)

	if vals := ctx.Config.GetString("username"); vals != "" {
		username = &vals
	}

	hours := ctx.Config.GetInt("hours")
	if hours < 1 || hours > 72 {
		return fmt.Errorf("Invalid expiration time (1-72 hours)\n")
	}

	icert, err := client.IssueSSHCertificate(org, email.Address, username, &hours)
	if err != nil {
		return err
	}

	doAgent := ctx.Config.GetBool("agent")
	if doAgent {
		if err = populateAgent(icert); err != nil {
			return err
		}

		fmt.Printf("Populated agent with cert:\n%s\n", icert.Certificate)
		return nil
	}

	pk, err := parsePrivateKey(icert.Key)
	if err != nil {
		return err
	}

	fmt.Printf(`
!!!! WARNING: We're now prompting you to save an SSH private key and certificate       !!!! 	
!!!! (the private key in "id_whatever" and the certificate in "id_whatever-cert.pub"). !!!! 	
!!!! These SSH credentials are time-limited and handling them in files is clunky;      !!!! 	
!!!! consider running an SSH agent and running this command with --agent. Things       !!!! 	
!!!! should just sort of work like magic if you do.                                    !!!!
`)

	var (
		rootname string
		pf       *os.File
		cf       *os.File
	)

	for pf == nil && cf == nil {
		prompt := "Path to store private key: "
		rootname, err = argOrPromptLoop(ctx, 2, prompt, rootname)
		if err != nil {
			return err
		}

		if ctx.Config.GetBool("dotssh") {
			rootname = fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), rootname)
		}

		mode := os.O_WRONLY | os.O_TRUNC | os.O_CREATE
		if !ctx.Config.GetBool("overwrite") {
			mode |= os.O_EXCL
		} else {
			if _, err = os.Stat(rootname); err == nil {
				if buf, err := ioutil.ReadFile(rootname); err != nil {
					ctx.Statusf("ssh", cmdctx.SERROR, "File exists, but we can't read it to make sure it's safe to overwrite: %s\n", err)
					continue
				} else if !strings.Contains(string(buf), "fly.io" /* BUG(tqbf): do better */) {
					ctx.Statusf("ssh", cmdctx.SERROR, "File exists, but isn't a fly.io ed25519 private key\n")
					continue
				}
			}
		}

		pf, err = os.OpenFile(rootname, mode, 0600)
		if err != nil {
			ctx.Statusf("ssh", cmdctx.SERROR, "Can't open private key file: %s\n", err)
			continue
		}

		cf, err = os.OpenFile(rootname+"-cert.pub", mode, 0600)
		if err != nil {
			pf.Close()
			ctx.Statusf("ssh", cmdctx.SERROR, "Can't open certificate file %s: %s", rootname+"-cert.pub", err)
		}
	}

	io.WriteString(cf, icert.Certificate)
	cf.Close()

	buf := MarshalED25519PrivateKey(pk, "fly.io")
	pf.Write(buf)
	pf.Close()

	fmt.Printf("Wrote %d-hour SSH credential to %s, %s-cert.pub\n", hours, rootname, rootname)

	return nil
}

func parsePrivateKey(key64 string) (ed25519.PrivateKey, error) {
	pkeys, err := base64.StdEncoding.DecodeString(key64)
	if err != nil {
		return nil, fmt.Errorf("API error: can't parse API-provided private key: %w", err)
	}
	return ed25519.NewKeyFromSeed(pkeys), nil
}

func populateAgent(icert *api.IssuedCertificate) error {
	acon, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return fmt.Errorf("can't connect to SSH agent: %w", err)
	}

	ssha := agent.NewClient(acon)

	cert, err := sshcert.ParsePublicKey(icert.Certificate)
	if err != nil {
		return fmt.Errorf("API error: can't parse API-provided SSH certificate: %w", err)
	}

	pkey, err := parsePrivateKey(icert.Key)
	if err != nil {
		return err
	}

	if err = ssha.Add(agent.AddedKey{
		PrivateKey:  pkey,
		Certificate: cert.(*ssh.Certificate),
	}); err != nil {
		return fmt.Errorf("ssh-agent failure: %w", err)
	}

	return nil
}

// stolen from `mikesmitty`, thanks, you are a mikesmitty and a scholar
func MarshalED25519PrivateKey(key ed25519.PrivateKey, comment string) []byte {
	magic := append([]byte("openssh-key-v1"), 0)

	var w struct {
		CipherName   string
		KdfName      string
		KdfOpts      string
		NumKeys      uint32
		PubKey       []byte
		PrivKeyBlock []byte
	}

	pk1 := struct {
		Check1  uint32
		Check2  uint32
		Keytype string
		Pub     []byte
		Priv    []byte
		Comment string
		Pad     []byte `ssh:"rest"`
	}{}

	ci := rand.Uint32()
	pk1.Check1 = ci
	pk1.Check2 = ci

	pk1.Keytype = ssh.KeyAlgoED25519

	pk := key.Public().(ed25519.PublicKey)
	pubKey := []byte(pk)
	pk1.Pub = pubKey
	pk1.Priv = []byte(key)
	pk1.Comment = comment

	// Add some padding to match the encryption block size within PrivKeyBlock (without Pad field)
	// 8 doesn't match the documentation, but that's what ssh-keygen uses for unencrypted keys. *shrug*
	bs := 8
	blockLen := len(ssh.Marshal(pk1))
	padLen := (bs - (blockLen % bs)) % bs
	pk1.Pad = make([]byte, padLen)

	for i := 0; i < padLen; i++ {
		pk1.Pad[i] = byte(i + 1)
	}

	// Generate the pubkey prefix "\0\0\0\nssh-ed25519\0\0\0 "
	prefix := []byte{0x0, 0x0, 0x0, 0x0b}
	prefix = append(prefix, []byte(ssh.KeyAlgoED25519)...)
	prefix = append(prefix, []byte{0x0, 0x0, 0x0, 0x20}...)

	w.CipherName = "none"
	w.KdfName = "none"
	w.KdfOpts = ""
	w.NumKeys = 1
	w.PubKey = append(prefix, pubKey...)
	w.PrivKeyBlock = ssh.Marshal(pk1)

	magic = append(magic, ssh.Marshal(w)...)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: magic,
	})
}
