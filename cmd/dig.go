package cmd

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
)

func newDigCommand(client *client.Client) *Command {
	cmd := BuildCommandKS(nil, runDig,
		docstrings.Get("dig"), client, requireSession)
	cmd.Args = cobra.RangeArgs(1, 2)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Shorthand:   "o",
		Default:     "",
		Description: "Select organization for DNS lookups instead of current app",
	})

	return cmd
}

func ResolverForOrg(c *agent.Client, org *api.Organization) (*net.Resolver, error) {
	// do this explicitly so we can get the DNS server address
	ts, err := c.Establish(context.Background(), org.Slug)
	if err != nil {
		return nil, err
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d, err := c.Dialer(ctx, org)
			if err != nil {
				return nil, err
			}

			network = "tcp"
			server := net.JoinHostPort(ts.TunnelConfig.DNS.String(), "53")

			// the connections we get from the agent are over a unix domain socket proxy,
			// which implements the PacketConn interface, so Go's janky DNS library thinks
			// we want UDP DNS. Trip it up.
			type fakeConn struct {
				net.Conn
			}

			c, err := d.DialContext(ctx, network, server)
			if err != nil {
				return nil, err
			}

			return &fakeConn{c}, nil
		},
	}, nil
}

func runDig(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()
	client := cmdCtx.Client.API()

	var (
		org *api.Organization
		err error
	)

	orgSlug := cmdCtx.Config.GetString("org")
	if orgSlug == "" {
		app, err := client.GetApp(ctx, cmdCtx.AppName)
		if err != nil {
			return fmt.Errorf("get app: %w", err)
		}

		org = &app.Organization
	} else {
		org, err = client.FindOrganizationBySlug(ctx, orgSlug)
		if err != nil {
			return fmt.Errorf("look up org: %w", err)
		}
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return err
	}

	r, err := ResolverForOrg(agentclient, org)
	if err != nil {
		return err
	}

	dtype := "aaaa"
	name := cmdCtx.Args[0]

	if len(cmdCtx.Args) > 1 {
		dtype = strings.ToLower(cmdCtx.Args[0])
		name = cmdCtx.Args[1]
	}

	switch dtype {
	case "aaaa":
		hosts, err := r.LookupHost(ctx, name)
		if err != nil {
			return err
		}

		for _, h := range hosts {
			fmt.Printf("%s\n", h)
		}

	case "txt":
		txts, err := r.LookupTXT(ctx, name)
		if err != nil {
			return err
		}

		fmt.Printf("%s\n", strings.Join(txts, ""))

	default:
		return fmt.Errorf("don't understand DNS type %s", dtype)
	}

	return nil
}
