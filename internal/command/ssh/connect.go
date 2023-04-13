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
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/ssh"
	"github.com/superfly/flyctl/terminal"
)

const DefaultSshUsername = "root"

func BringUpAgent(ctx context.Context, client *api.Client, app *api.AppCompact, quiet bool) (*agent.Client, agent.Dialer, error) {
	io := iostreams.FromContext(ctx)

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		captureError(err, app)
		return nil, nil, errors.Wrap(err, "can't establish agent")
	}

	dialer, err := agentclient.Dialer(ctx, app.Organization.Slug)
	if err != nil {
		captureError(err, app)
		return nil, nil, fmt.Errorf("ssh: can't build tunnel for %s: %s\n", app.Organization.Slug, err)
	}

	if !quiet {
		io.StartProgressIndicatorMsg("Connecting to tunnel")
	}
	if err := agentclient.WaitForTunnel(ctx, app.Organization.Slug); err != nil {
		captureError(err, app)
		return nil, nil, errors.Wrapf(err, "tunnel unavailable")
	}
	if !quiet {
		io.StopProgressIndicator()
	}

	return agentclient, dialer, nil
}

type ConnectParams struct {
	Ctx            context.Context
	Org            api.OrganizationImpl
	Username       string
	Dialer         agent.Dialer
	DisableSpinner bool
}

func Connect(p *ConnectParams, addr string) (*ssh.Client, error) {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cert, pk, err := singleUseSSHCertificate(p.Ctx, p.Org)
	if err != nil {
		return nil, fmt.Errorf("create ssh certificate: %w (if you haven't created a key for your org yet, try `flyctl ssh issue`)", err)
	}

	pemkey := ssh.MarshalED25519PrivateKey(pk, "single-use certificate")

	terminal.Debugf("Keys for %s configured; connecting...\n", addr)

	sshClient := &ssh.Client{
		Addr: net.JoinHostPort(addr, "22"),
		User: p.Username,

		Dial: p.Dialer.DialContext,

		Certificate: cert.Certificate,
		PrivateKey:  string(pemkey),
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

	terminal.Debugf("Connection completed.\n", addr)

	if !p.DisableSpinner {
		endSpin()
	}

	return sshClient, nil
}

func singleUseSSHCertificate(ctx context.Context, org api.OrganizationImpl) (*api.IssuedCertificate, ed25519.PrivateKey, error) {
	client := client.FromContext(ctx).API()
	hours := 1

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, nil, err
	}

	icert, err := client.IssueSSHCertificate(ctx, org, []string{DefaultSshUsername, "fly"}, nil, &hours, pub)
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
