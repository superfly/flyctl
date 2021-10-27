package recipe

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/pkg/agent"
	flySsh "github.com/superfly/flyctl/pkg/ssh"
	"github.com/superfly/flyctl/terminal"

	"golang.org/x/crypto/ssh"
)

type SSHParams struct {
	Ctx       context.Context
	Org       *api.Organization
	App       string
	Dialer    agent.Dialer
	ApiClient *api.Client
	Cmd       string
	Stdin     io.Reader
	Stdout    io.WriteCloser
	Stderr    io.WriteCloser
}

func sshConnect(p *SSHParams, addr string) error {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cert, err := singleUseSSHCertificate(p.Ctx, p.ApiClient, p.Org)
	if err != nil {
		return fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh establish`)", err)
	}

	pk, err := parsePrivateKey(cert.Key)
	if err != nil {
		return errors.Wrap(err, "parse ssh certificate")
	}

	pemkey := MarshalED25519PrivateKey(pk, "single-use certificate")

	terminal.Debugf("Keys for %s configured; connecting...\n", addr)

	sshClient := &flySsh.Client{
		Addr: addr + ":22",
		User: "root",

		Dial: p.Dialer.DialContext,

		Certificate: cert.Certificate,
		PrivateKey:  string(pemkey),
	}

	if err := sshClient.Connect(context.Background()); err != nil {
		return errors.Wrap(err, "error connecting to SSH server")
	}
	defer sshClient.Close()

	terminal.Debugf("Connection completed.\n", addr)

	term := &flySsh.Terminal{
		Stdin:  p.Stdin,
		Stdout: p.Stdout,
		Stderr: p.Stderr,
		Mode:   "xterm",
	}

	if err := sshClient.Shell(context.Background(), term, p.Cmd); err != nil {
		return errors.Wrap(err, "ssh shell")
	}

	return nil
}

func singleUseSSHCertificate(ctx context.Context, apiClient *api.Client, org *api.Organization) (*api.IssuedCertificate, error) {
	user, err := apiClient.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	hours := 1
	return apiClient.IssueSSHCertificate(ctx, org, user.Email, nil, &hours)
}

func parsePrivateKey(key64 string) (ed25519.PrivateKey, error) {
	pkeys, err := base64.StdEncoding.DecodeString(key64)
	if err != nil {
		return nil, fmt.Errorf("API error: can't parse API-provided private key: %w", err)
	}
	return ed25519.NewKeyFromSeed(pkeys), nil
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

func spin(in, out string) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	if !helpers.IsTerminal() {
		fmt.Fprintln(os.Stderr, in)
		return cancel
	}

	go func() {
		s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		s.Writer = os.Stderr
		s.Prefix = in
		s.FinalMSG = out
		s.Start()
		defer s.Stop()

		<-ctx.Done()
	}()

	return cancel
}
