package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/pkg/wg"
)

func newProxyCommand() *Command {
	cmd := BuildCommandKS(nil, runProxy, docstrings.Get("proxy"), os.Stdout, requireSession, requireAppName)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runProxy(ctx *cmdctx.CmdContext) error {
	client := ctx.Client.API()

	app, err := client.GetApp(ctx.AppConfig.AppName)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	state, err := wireGuardForOrg(ctx, &app.Organization)
	if err != nil {
		return fmt.Errorf("create wireguard config: %w", err)
	}

	tunnel, err := wg.Connect(*state.TunnelConfig())
	if err != nil {
		return fmt.Errorf("connect wireguard: %w", err)
	}

	addr, err := argOrPrompt(ctx, 0, "Address to proxy to:")
	if err != nil {
		return fmt.Errorf("get address to proxy to: %w", err)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse address: %w", err)
	}

	if host == "" {
		host = ctx.AppConfig.AppName + ".internal"
	}

	_, err = tunnel.Resolver().LookupHost(context.Background(), host)
	if err != nil {
		return fmt.Errorf("look up %s: %w", host, err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for local connections: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Listening for connections on %s\n", listener.Addr())

	for {
		in, err := listener.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed incoming connection: %s\n", err)
			continue
		}

		go func() {
			out, err := tunnel.DialContext(context.Background(), "tcp", net.JoinHostPort(host, port))
			if err != nil {
				in.Close()
				fmt.Fprintf(os.Stderr, "Can't open outbound connection to %s:%d: %s\n", host, port, err)
				return
			}

			defer out.Close()
			defer in.Close()

			fmt.Fprintf(os.Stderr, "Connected %s to %s\n", in.RemoteAddr(), out.RemoteAddr())

			wg := &sync.WaitGroup{}
			wg.Add(2)

			go func() {
				defer wg.Done()
				io.Copy(in, out)
			}()

			go func() {
				defer wg.Done()
				io.Copy(out, in)
			}()

			wg.Wait()
		}()
	}
}
