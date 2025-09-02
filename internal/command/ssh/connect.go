package ssh

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/pkg/errors"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/ssh"
	"github.com/superfly/flyctl/terminal"
)

const DefaultSshUsername = "root"

type ConnectParams struct {
	Ctx            context.Context
	Org            fly.OrganizationImpl
	Username       string
	Dialer         agent.Dialer
	DisableSpinner bool
	Container      string
	AppNames       []string
}

func Connect(p *ConnectParams, addr string) (*ssh.Client, error) {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cacheKey := fmt.Sprint(p.Username, "@", p.AppNames)
	key, err := flyutil.FetchCertificate(p.Ctx, cacheKey, 1*time.Hour, func() (*fly.IssuedCertificate, error) {
		cert, pk, err := singleUseSSHCertificate(p.Ctx, p.Org, p.AppNames, p.Username)
		if err != nil {
			return nil, fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh issue`)", err)
		}
		pemkey := ssh.MarshalED25519PrivateKey(pk, "single-use certificate")
		cert.Key = string(pemkey)
		return cert, nil
	})
	if err != nil {
		return nil, err
	}

	terminal.Debugf("Keys for %s configured; connecting...\n", addr)

	sshClient := &ssh.Client{
		Addr: net.JoinHostPort(addr, "22"),
		User: p.Username,

		Dial: p.Dialer.DialContext,

		Certificate: key.Certificate,
		PrivateKey:  key.Key,
	}

	var endSpin context.CancelFunc
	if !p.DisableSpinner {
		endSpin = spin(fmt.Sprintf("Connecting to %s...", addr),
			fmt.Sprintf("Connecting to %s... complete\n", addr))
		defer endSpin()
	}

	if err := sshClient.Connect(p.Ctx); err != nil {
		return nil, errors.Wrap(err, "error connecting to SSH server")
	}

	terminal.Debugf("Connection %s completed.\n", addr)

	if !p.DisableSpinner {
		endSpin()
	}

	return sshClient, nil
}

func singleUseSSHCertificate(ctx context.Context, org fly.OrganizationImpl, appNames []string, user string) (*fly.IssuedCertificate, ed25519.PrivateKey, error) {
	client := flyutil.ClientFromContext(ctx)
	hours := 1

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, err
	}

	icert, err := client.IssueSSHCertificate(ctx, org, []string{user, "fly"}, appNames, &hours, pub)
	if err != nil {
		return nil, nil, err
	}

	return icert, priv, nil
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
