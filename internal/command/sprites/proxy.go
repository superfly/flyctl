package sprites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/iostreams"
)

func newProxy() *cobra.Command {
	const (
		short = "Forward local ports to a sprite."

		long = `Forward TCP ports on localhost to ports in a sprite, for as long as the
command runs. This works with any TCP protocol, such as databases or dev
servers. For always-on HTTP access, use the sprite's URL instead: see
'fly sprites url'.

Each argument is a port, forwarded to the same port in the sprite, or
<local>:<remote> to use different ports.

--stdio connects stdin and stdout to a single port in the sprite. --ssh serves
an SSH session with the sprite over stdin and stdout, for use as an OpenSSH
ProxyCommand:

  Host my-sprite
    ProxyCommand fly sprites proxy --ssh -s my-sprite
`
	)

	cmd := newCommand("proxy [flags] <port|local:remote>...", short, long, runProxy)
	cmd.Example = `  fly sprites proxy 8080
  fly sprites proxy 3001:3000 8081:8080
  fly sprites proxy -W :22
  fly sprites proxy --ssh -s my-sprite`

	flag.Add(cmd,
		flag.String{
			Name:        "stdio",
			Shorthand:   "W",
			Description: "Forward stdin and stdout to [host]:port in the sprite",
		},
		flag.Bool{
			Name:        "ssh",
			Description: "Serve an SSH session with the sprite over stdio, for use as an ssh ProxyCommand",
		},
	)

	return cmd
}

func runProxy(ctx context.Context, s *session) error {
	var (
		args  = flag.Args(ctx)
		stdio = flag.GetString(ctx, "stdio")
		ssh   = flag.GetBool(ctx, "ssh")
	)

	switch {
	case ssh && stdio != "":
		return errors.New("--ssh and --stdio cannot be used together")
	case (ssh || stdio != "") && len(args) > 0:
		return errors.New("ports cannot be combined with --ssh or --stdio")
	case !ssh && stdio == "" && len(args) == 0:
		return errors.New("at least one port is required")
	}

	// Parse before resolving the sprite, so a typo fails without a prompt.
	mappings, err := parsePortMappings(args)
	if err != nil {
		return err
	}

	sprite, err := s.sprite(ctx, "")
	if err != nil {
		return err
	}

	switch {
	case ssh:
		return proxySSH(ctx, s, sprite)
	case stdio != "":
		return proxyStdio(ctx, sprite, stdio)
	default:
		return proxyPorts(ctx, sprite, mappings)
	}
}

// parsePortMappings parses "port" and "local:remote" arguments.
func parsePortMappings(args []string) ([]sprites.PortMapping, error) {
	mappings := make([]sprites.PortMapping, 0, len(args))

	for _, arg := range args {
		local, remote, split := strings.Cut(arg, ":")
		if !split {
			remote = local
		}

		localPort, err := parsePort(local)
		if err != nil {
			return nil, fmt.Errorf("invalid port mapping %q: %w", arg, err)
		}

		remotePort, err := parsePort(remote)
		if err != nil {
			return nil, fmt.Errorf("invalid port mapping %q: %w", arg, err)
		}

		mappings = append(mappings, sprites.PortMapping{LocalPort: localPort, RemotePort: remotePort})
	}

	return mappings, nil
}

func parsePort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%q is not a port between 1 and 65535", s)
	}

	return port, nil
}

func proxyPorts(ctx context.Context, sprite *sprites.Sprite, mappings []sprites.PortMapping) error {
	ios := iostreams.FromContext(ctx)

	forwards, err := sprite.ProxyPorts(ctx, mappings)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return flyerr.WithSuggestion(
				fmt.Errorf("failed to start proxy: %w", err),
				"A local port is already in use. Stop whatever is using it, including a 'fly sprites exec' or "+
					"'console' that auto-forwarded it, or map a different local port with <local>:<remote>.",
			)
		}

		return fmt.Errorf("failed to start proxy: %w", err)
	}

	defer func() {
		for _, forward := range forwards {
			_ = forward.Close()
		}
	}()

	for i, forward := range forwards {
		fmt.Fprintf(ios.Out, "Forwarding %s -> %s port %d\n", forward.LocalAddr(), sprite.Name(), mappings[i].RemotePort)
	}

	fmt.Fprintln(ios.Out, "Press Ctrl+C to stop.")

	<-ctx.Done()

	return nil
}

// proxyStdio connects stdin and stdout to one address in the sprite.
func proxyStdio(ctx context.Context, sprite *sprites.Sprite, addr string) error {
	ios := iostreams.FromContext(ctx)

	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	conn, err := sprite.ProxySocket(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s in sprite %s: %w", addr, sprite.Name(), err)
	}

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		_, err := io.Copy(conn, os.Stdin)
		return err
	})

	g.Go(func() error {
		_, err := io.Copy(ios.Out, conn)

		// The remote side closing ends the session, even though stdin is
		// still open.
		if err == nil {
			err = io.EOF
		}

		return err
	})

	stop := context.AfterFunc(gctx, func() { _ = conn.Close() })
	defer stop()

	if err := g.Wait(); err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
		return err
	}

	return nil
}
