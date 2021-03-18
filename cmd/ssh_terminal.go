package cmd

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/briandowns/spinner"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/pkg/ssh"
	"github.com/superfly/flyctl/pkg/wg"
	"github.com/superfly/flyctl/terminal"
)

// this is going away; it's runSSHConsole with crappier args
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

	return sshConnect(&SSHParams{
		Ctx:    ctx,
		Org:    org,
		Tunnel: tunnel,
	}, addr)
}

func runSSHConsole(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	terminal.Debugf("Retrieving app info for %s\n", ctx.AppConfig.AppName)

	app, err := client.GetApp(ctx.AppConfig.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	state, err := wireGuardForOrg(ctx, &app.Organization)
	if err != nil {
		return fmt.Errorf("create wireguard config: %w", err)
	}

	terminal.Debugf("Establishing WireGuard connection (%s)\n", state.Name)

	tunnel, err := wg.Connect(*state.TunnelConfig())
	if err != nil {
		return fmt.Errorf("connect wireguard: %w", err)
	}

	if ctx.Config.GetBool("probe") {
		if err = probeConnection(tunnel.Resolver()); err != nil {
			return fmt.Errorf("probe wireguard: %w", err)
		}
	}

	var addr string

	if ctx.Config.GetBool("select") {
		instances, err := allInstances(tunnel.Resolver(), ctx.AppConfig.AppName)
		if err != nil {
			return fmt.Errorf("look up %s: %w", ctx.AppConfig.AppName, err)
		}

		selected := 0
		prompt := &survey.Select{
			Message:  "Select instance:",
			Options:  instances.Labels,
			PageSize: 15,
		}

		if err := survey.AskOne(prompt, &selected); err != nil {
			return fmt.Errorf("selecting instance: %w", err)
		}

		addr = fmt.Sprintf("[%s]", instances.Addresses[selected])
	} else if len(ctx.Args) != 0 {
		addr = ctx.Args[0]
	} else {
		addr = fmt.Sprintf("%s.internal", ctx.AppConfig.AppName)
	}

	return sshConnect(&SSHParams{
		Ctx:    ctx,
		Org:    &app.Organization,
		Tunnel: tunnel,
		App:    ctx.AppConfig.AppName,
	}, addr)
}

type Instances struct {
	Labels    []string
	Addresses []string
}

func allInstances(r *net.Resolver, app string) (*Instances, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endSpin := spin("Looking up regions in DNS...", "Looking up regions in DNS... complete!\n")
	defer endSpin()

	regionsv, err := r.LookupTXT(ctx, fmt.Sprintf("regions.%s.internal", app))
	if err != nil {
		return nil, fmt.Errorf("look up regions for %s: %w", app, err)
	}

	regions := strings.Trim(regionsv[0], " \t")
	if regions == "" {
		return nil, fmt.Errorf("can't find deployed regions for %s", app)
	}

	ret := &Instances{}

	for _, region := range strings.Split(regions, ",") {
		name := fmt.Sprintf("%s.%s.internal", region, app)
		addrs, err := r.LookupHost(ctx, name)
		if err != nil {
			log.Printf("can't lookup records for %s: %s", name, err)
			continue
		}

		if len(addrs) == 1 {
			ret.Labels = append(ret.Labels, name)
			ret.Addresses = append(ret.Addresses, addrs[0])
			continue
		}

		for _, addr := range addrs {
			ret.Labels = append(ret.Labels, fmt.Sprintf("%s (%s)", region, addr))
			ret.Addresses = append(ret.Addresses, addrs[0])
		}
	}

	if len(ret.Addresses) == 0 {
		return nil, fmt.Errorf("no running hosts for %s found", app)
	}

	return ret, nil
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
		return
	}()

	return cancel
}

type SSHParams struct {
	Ctx    *cmdctx.CmdContext
	Org    *api.Organization
	App    string
	Tunnel *wg.Tunnel
}

func sshConnect(p *SSHParams, addr string) error {
	terminal.Debugf("Fetching certificate for %s\n", addr)

	cert, err := singleUseSSHCertificate(p.Ctx, p.Org)
	if err != nil {
		return fmt.Errorf("create ssh certificate: %w", err)
	}

	pk, err := parsePrivateKey(cert.Key)
	if err != nil {
		return fmt.Errorf("parse ssh certificate: %w", err)
	}

	pemkey := MarshalED25519PrivateKey(pk, "single-use certificate")

	terminal.Debugf("Keys for %s configured; connecting...\n", addr)

	sshClient := &ssh.Client{
		Addr: addr + ":22",
		User: "root",

		Dial: p.Tunnel.DialContext,

		Certificate: cert.Certificate,
		PrivateKey:  string(pemkey),
	}

	endSpin := spin(fmt.Sprintf("Connecting to %s...", addr),
		fmt.Sprintf("Connecting to %s... complete\n", addr))
	defer endSpin()

	if err := sshClient.Connect(context.Background()); err != nil {
		return fmt.Errorf("connect to SSH server: %w", err)
	}
	defer sshClient.Close()

	terminal.Debugf("Connection completed.\n", addr)

	endSpin()

	term := &ssh.Terminal{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Mode:   "xterm",
	}

	if err := sshClient.Shell(context.Background(), term); err != nil {
		return fmt.Errorf("SSH shell: %w", err)
	}

	return nil
}

func probeConnection(r *net.Resolver) error {
	var (
		err error
		res []string
	)

	for i := 0; i < 3; i++ {
		terminal.Debugf("Probing WireGuard connectivity, attempt %d\n", i)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		res, err = r.LookupTXT(ctx, fmt.Sprintf("_apps.internal"))

		cancel()

		if err == nil {
			break
		}
	}

	if err != nil {
		return fmt.Errorf("look up apps: %w", err)
	}

	terminal.Debugf("Found _apps.internal TXT: %+v\n", res)

	return nil
}
