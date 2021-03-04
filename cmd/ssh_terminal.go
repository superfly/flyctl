package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/pkg/ssh"
	"github.com/superfly/flyctl/pkg/wg"
)

func runSSHShell(ctx *cmdctx.CmdContext) error {
	org, err := orgByArg(ctx)
	if err != nil {
		return err
	}

	state, err := wireGuardForOrg(ctx, org)
	if err != nil {
		return err
	}

	addr, err := argOrPrompt(ctx, 1, "Host to connect to: ")
	if err != nil {
		return err
	}

	tunnel, err := wg.Connect(*state.TunnelConfig())
	if err != nil {
		return err
	}

	if n := net.ParseIP(addr); n != nil && n.To16() != nil {
		addr = fmt.Sprintf("[%s]", addr)
	} else if strings.Contains(addr, ".internal") {
		addrs, err := tunnel.Resolver().LookupHost(context.Background(),
			addr)
		if err != nil {
			return err
		}

		addr = fmt.Sprintf("[%s]", addrs[0])
	}

	cert, err := singleUseSSHCertificate(ctx, org)
	if err != nil {
		return err
	}

	pk, err := parsePrivateKey(cert.Key)
	if err != nil {
		return err
	}

	pemkey := MarshalED25519PrivateKey(pk, "single-use certificate")

	sshClient := &ssh.Client{
		Addr: addr + ":22",
		User: "root",

		Dial: tunnel.DialContext,

		Certificate: cert.Certificate,
		PrivateKey:  string(pemkey),
	}

	if err := sshClient.Connect(context.Background()); err != nil {
		return err
	}
	defer sshClient.Close()

	term := &ssh.Terminal{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Mode:   "xterm",
	}

	if err := sshClient.Shell(context.Background(), term); err != nil {
		return err
	}

	return nil
}
