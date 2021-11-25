package cmd

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/miekg/dns"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/agent"
)

func newDigCommand(client *client.Client) *Command {
	cmd := BuildCommandKS(nil, runDig,
		docstrings.Get("dig"), client, requireSession, optionalAppName)
	cmd.Args = cobra.RangeArgs(1, 2)

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Shorthand:   "o",
		Default:     "",
		Description: "Select organization for DNS lookups instead of current app",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "short",
		Shorthand:   "s",
		Default:     false,
		Description: "Just print the answers, not DNS record details",
	})

	return cmd
}

var (
	nameErrorRx = regexp.MustCompile(`\[.*?\]:53`)
)

// FixNameOrError cleans up resolver errors; the Go stdlib doesn't notice when
// you swap out the host its resolver connects to, and prints the resolv.conf
// resolver in error messages, which is super confusing for users.
func FixNameError(err error, ns string) error {
	if err == nil {
		return err
	}

	str := nameErrorRx.ReplaceAllString(err.Error(), fmt.Sprintf("[%s]:53", ns))
	return fmt.Errorf(str) // gross but whatever
}

// ResolverForOrg takes a connection to the wireguard agent and an organization
// and returns a working net.Resolver for DNS for that organization, along with the
// address of the nameserver.
func ResolverForOrg(c *agent.Client, org *api.Organization) (*net.Resolver, string, error) {
	// do this explicitly so we can get the DNS server address
	ts, err := c.Establish(context.Background(), org.Slug)
	if err != nil {
		return nil, "", err
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
	}, ts.TunnelConfig.DNS.String(), nil
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

	r, ns, err := ResolverForOrg(agentclient, org)
	if err != nil {
		return err
	}

	d, err := agentclient.Dialer(ctx, org)
	if err != nil {
		return err
	}

	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ns, "53"))
	if err != nil {
		return err
	}

	msg := &dns.Msg{}

	dtype := "AAAA"
	name := cmdCtx.Args[0]

	if len(cmdCtx.Args) > 1 {
		dtype = strings.ToUpper(cmdCtx.Args[0])
		name = cmdCtx.Args[1]
	}

	// round trip a DNS request across a "TCP" socket; we'd just
	// use miekg/dns's Client, but I don't think it promises to
	// work over our weird UDS TCP proxy.
	rtrip := func(m *dns.Msg) (*dns.Msg, error) {
		m.Id = dns.Id()
		m.Compress = true

		buf, err := m.Pack()
		if err != nil {
			return nil, fmt.Errorf("dns round trip: %w", err)
		}

		var lenbuf [2]byte
		binary.BigEndian.PutUint16(lenbuf[:], uint16(len(buf)))

		if _, err = conn.Write(lenbuf[:]); err != nil {
			return nil, fmt.Errorf("dns round trip: %w", err)
		}

		if _, err = conn.Write(buf); err != nil {
			return nil, fmt.Errorf("dns round trip: %w", err)
		}

		if _, err = conn.Read(lenbuf[:]); err != nil {
			return nil, fmt.Errorf("dns round trip: %w", err)
		}

		l := int(binary.BigEndian.Uint16(lenbuf[:]))
		buf = make([]byte, l)

		if _, err = conn.Read(buf); err != nil {
			return nil, fmt.Errorf("dns round trip: %w", err)
		}

		ret := &dns.Msg{}
		if err = ret.Unpack(buf); err != nil {
			return nil, fmt.Errorf("dns round trip: %w", err)
		}

		return ret, nil
	}

	// add the trailing dot
	name = dns.Fqdn(name)

	if strings.HasSuffix(name, ".internal.") {
		msg.RecursionDesired = false
	} else {
		msg.RecursionDesired = true
	}

	switch dtype {
	case "A":
		fallthrough
	case "CNAME":
		fallthrough
	case "TXT":
		fallthrough
	case "AAAA":
		msg.SetQuestion(name, dns.StringToType[dtype])

		reply, err := rtrip(msg)
		if err != nil {
			return err
		}

		if cmdCtx.Config.GetBool("short") {
			if reply.MsgHdr.Rcode != dns.RcodeSuccess {
				return fmt.Errorf("lookup failed: %s", dns.RcodeToString[reply.MsgHdr.Rcode])
			}

			switch dtype {
			case "AAAA":
				for _, rr := range reply.Answer {
					if aaaa, ok := rr.(*dns.AAAA); ok {
						fmt.Printf("%s\n", aaaa.AAAA)
					}
				}
			case "TXT":
				buf := &bytes.Buffer{}

				for _, rr := range reply.Answer {
					if txt, ok := rr.(*dns.TXT); ok {
						for _, s := range txt.Txt {
							buf.WriteString(s)
						}
					}
				}

				fmt.Printf("%s\n", buf.String())
			}
		} else {
			fmt.Printf("%+v\n", reply)
		}

	case "AAAA-NATIVE":
		hosts, err := r.LookupHost(ctx, name)
		if err != nil {
			return FixNameError(err, ns)
		}

		for _, h := range hosts {
			fmt.Printf("%s\n", h)
		}

	case "TXT-NATIVE":
		txts, err := r.LookupTXT(ctx, name)
		if err != nil {
			return FixNameError(err, ns)
		}

		fmt.Printf("%s\n", strings.Join(txts, ""))

	default:
		return fmt.Errorf("don't understand DNS type %s", dtype)
	}

	return nil
}
