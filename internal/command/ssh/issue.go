package ssh

import (
	"context"
	"crypto/ed25519"
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/mail"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/ejcx/sshcert"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newIssue() *cobra.Command {
	const (
		long = `Issue a new SSH credential. With -agent, populate credential
into SSH agent. With -hour, set the number of hours (1-72) for credential
validity.`
		short = `Issue a new SSH credential`
		usage = "issue [org] [email] [path]"
	)

	cmd := command.New(usage, short, long, runSSHIssue, command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(3)

	flag.Add(cmd,
		flag.Org(),
		flag.String{
			Name:        "username",
			Shorthand:   "u",
			Description: "Unix username for SSH cert",
		},
		flag.Int{
			Name:        "hours",
			Default:     24,
			Description: "Expiration, in hours (<72)",
		},

		flag.Bool{
			Name:        "agent",
			Default:     false,
			Description: "Add key to SSH agent",
		},
		flag.Bool{
			Name:        "dotssh",
			Shorthand:   "d",
			Default:     false,
			Description: "Store keys in ~/.ssh, like normal keys",
		},

		flag.Bool{
			Name:        "overwrite",
			Default:     false,
			Description: "Overwrite existing SSH keys in same location, if we generated them",
		},
	)

	return cmd
}

func runSSHIssue(ctx context.Context) (err error) {

	client := client.FromContext(ctx).API()
	out := iostreams.FromContext(ctx).Out

	org, err := orgs.OrgFromFirstArgOrSelect(ctx)

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
			fmt.Fprintf(out, "Invalid email address: %s (keep it simple!)\n", err)
			email = nil
		}
	}

	var (
		username *string
	)

	if vals := flag.GetString(ctx, "username"); vals != "" {
		username = &vals
	}

	hours := flag.GetInt(ctx, "hours")
	if hours < 1 || hours > 72 {
		return fmt.Errorf("Invalid expiration time (1-72 hours)\n")
	}

	icert, err := client.IssueSSHCertificate(ctx, org, email.Address, username, &hours)
	if err != nil {
		return err
	}

	doAgent := flag.GetBool(ctx, "agent")
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

		if flag.GetBool(ctx, "dotssh") {
			rootname = fmt.Sprintf("%s/.ssh/%s", os.Getenv("HOME"), rootname)
		}

		mode := os.O_WRONLY | os.O_TRUNC | os.O_CREATE
		if !flag.GetBool(ctx, "overwrite") {
			mode |= os.O_EXCL
		} else if _, err = os.Stat(rootname); err == nil {
			if buf, err := ioutil.ReadFile(rootname); err != nil {
				fmt.Fprintf(out, "File exists, but we can't read it to make sure it's safe to overwrite: %s\n", err)
				continue
			} else if !strings.Contains(string(buf), "fly.io" /* BUG(tqbf): do better */) {
				fmt.Fprintf(out, "File exists, but isn't a fly.io ed25519 private key\n")
				continue
			}

		}

		pf, err = os.OpenFile(rootname, mode, 0600)
		if err != nil {
			fmt.Fprintf(out, "Can't open private key file: %s\n", err)
			continue
		}

		cf, err = os.OpenFile(rootname+"-cert.pub", mode, 0600)
		if err != nil {
			pf.Close()
			fmt.Fprintf(out, "Can't open certificate file %s: %s", rootname+"-cert.pub", err)
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

func argOrPromptImpl(ctx context.Context, nth int, prompt string, first bool) (string, error) {
	if len(flag.Args(ctx)) >= (nth + 1) {
		return flag.Args(ctx)[nth], nil
	}

	val := ""
	err := survey.AskOne(&survey.Input{
		Message: prompt,
	}, &val)

	return val, err
}

func argOrPromptLoop(ctx context.Context, nth int, prompt, last string) (string, error) {
	return argOrPromptImpl(ctx, nth, prompt, last == "")
}

func argOrPrompt(ctx context.Context, nth int, prompt string) (string, error) {
	return argOrPromptImpl(ctx, nth, prompt, true)
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
	w.PubKey = append(w.PubKey, prefix...)
	w.PubKey = append(w.PubKey, pubKey...)
	w.PrivKeyBlock = ssh.Marshal(pk1)

	magic = append(magic, ssh.Marshal(w)...)

	return pem.EncodeToMemory(&pem.Block{
		Type:  "OPENSSH PRIVATE KEY",
		Bytes: magic,
	})
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
