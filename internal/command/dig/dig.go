// Package dig implements the dig command chain.
package dig

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/miekg/dns"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

var (
	nameErrorRx = regexp.MustCompile(`\[.*?\]:53`)
)

func New() *cobra.Command {
	const (
		long = `Make DNS requests against Fly.io's internal DNS server. Valid types include
AAAA and TXT (the two types our servers answer authoritatively), AAAA-NATIVE
and TXT-NATIVE, which resolve with Go's resolver (they're slower,
but may be useful if diagnosing a DNS bug) and A and CNAME
(if you're using the server to test recursive lookups.)
Note that this resolves names against the server for the current organization. You can
set the organization with -o <org-slug>; otherwise, the command uses the organization
attached to the current app (you can pass an app in with -a <appname>).`

		short = "Make DNS requests against Fly.io's internal DNS server"
	)

	cmd := command.New("dig [type] <name> [flags]", short, long, run,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.RangeArgs(1, 2)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Bool{
			Name:        "short",
			Shorthand:   "s",
			Default:     false,
			Description: "Just print the answers, not DNS record details",
		},
	)

	return cmd
}

func run(ctx context.Context) error {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)

		err error
	)

	orgSlug := flag.GetOrg(ctx)

	if orgSlug == "" {
		appName := app.NameFromContext(ctx)

		app, err := client.GetAppBasic(ctx, appName)
		if err != nil {
			return fmt.Errorf("get app: %w", err)
		}
		orgSlug = app.Organization.Slug
	}

	agentclient, err := agent.Establish(ctx, client)
	if err != nil {
		return err
	}

	r, ns, err := ResolverForOrg(ctx, agentclient, orgSlug)
	if err != nil {
		return err
	}

	d, err := agentclient.Dialer(ctx, orgSlug)
	if err != nil {
		return err
	}

	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ns, "53"))
	if err != nil {
		return err
	}

	msg := &dns.Msg{}

	dtype := "AAAA"
	name := flag.FirstArg(ctx)

	if len(flag.Args(ctx)) > 1 {
		dtype = strings.ToUpper(flag.FirstArg(ctx))
		name = flag.Args(ctx)[1]
	}
	// add the trailing dot
	name = dns.Fqdn(name)

	if strings.HasSuffix(name, ".internal.") {
		msg.RecursionDesired = false
	} else {
		msg.RecursionDesired = true
	}

	// put this switch block in its own function to reduce the footprint of the main function
	// e.g: func resolve(ctx context.Context, r *net.Resolver, msg *dns.Msg, name string, dtype string)
	switch dtype {
	case "A", "CNAME", "TXT", "AAAA":
		msg.SetQuestion(name, dns.StringToType[dtype])

		reply, err := roundTrip(conn, msg)
		if err != nil {
			return err
		}

		if flag.GetBool(ctx, "short") {
			if reply.MsgHdr.Rcode != dns.RcodeSuccess {
				return fmt.Errorf("lookup failed: %s", dns.RcodeToString[reply.MsgHdr.Rcode])
			}

			switch dtype {
			case "AAAA":
				for _, rr := range reply.Answer {
					if aaaa, ok := rr.(*dns.AAAA); ok {
						fmt.Fprintf(io.Out, "%s\n", aaaa.AAAA)
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

				fmt.Fprintf(io.Out, "%s\n", buf.String())
			}
		} else {
			fmt.Fprintf(io.Out, "%+v\n", reply)
		}

	case "AAAA-NATIVE":
		hosts, err := r.LookupHost(ctx, name)
		if err != nil {
			return fixNameError(err, ns)
		}

		for _, h := range hosts {
			fmt.Fprintf(io.Out, "%s\n", h)
		}

	case "TXT-NATIVE":
		txts, err := r.LookupTXT(ctx, name)
		if err != nil {
			return fixNameError(err, ns)
		}

		fmt.Fprintf(io.Out, "%s\n", strings.Join(txts, ""))

	default:
		return fmt.Errorf("don't understand DNS type %s", dtype)
	}

	return nil
}

// roundTrip a DNS request across a "TCP" socket; we'd just use miekg/dns's Client, but I don't think it promises to
// work over our weird UDS TCP proxy.
func roundTrip(conn net.Conn, m *dns.Msg) (*dns.Msg, error) {
	m.Id = dns.Id()
	m.Compress = true

	buf, err := m.Pack()
	if err != nil {
		return nil, err
	}

	var lenbuf [2]byte
	binary.BigEndian.PutUint16(lenbuf[:], uint16(len(buf)))

	if _, err = conn.Write(lenbuf[:]); err != nil {
		return nil, err
	}

	if _, err = conn.Write(buf); err != nil {
		return nil, err
	}

	if _, err = conn.Read(lenbuf[:]); err != nil {
		return nil, err
	}

	l := int(binary.BigEndian.Uint16(lenbuf[:]))
	buf = make([]byte, l)

	if _, err = conn.Read(buf); err != nil {
		return nil, err
	}

	ret := &dns.Msg{}
	if err = ret.Unpack(buf); err != nil {
		return nil, err
	}

	return ret, nil
}

// ResolverForOrg takes a connection to the wireguard agent and an organization
// and returns a working net.Resolver for DNS for that organization, along with the
// address of the nameserver.
func ResolverForOrg(ctx context.Context, c *agent.Client, orgSlug string) (*net.Resolver, string, error) {
	// do this explicitly so we can get the DNS server address
	ts, err := c.Establish(ctx, orgSlug)
	if err != nil {
		return nil, "", err
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d, err := c.Dialer(ctx, orgSlug)
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

// FixNameOrError cleans up resolver errors; the Go stdlib doesn't notice when
// you swap out the host its resolver connects to, and prints the resolv.conf
// resolver in error messages, which is super confusing for users.
func fixNameError(err error, ns string) error {
	if err == nil {
		return err
	}
	return errors.New(nameErrorRx.ReplaceAllString(err.Error(), fmt.Sprintf("[%s]:53", ns)))
}
