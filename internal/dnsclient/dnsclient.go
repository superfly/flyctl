package dnsclient

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/terminal"
)

// Internal DnsClient, which queries both the graphql api and .internal dns to answer queries
// Graphql wins when there are differences, and those discrepancies are reported to Fly's error recording system
type DnsClient struct {
	dnsClient *dns.Client
	flyClient *client.Client
}

func DnsClientFromContext(ctx context.Context) *DnsClient {
	dnsClient := &dns.Client{
		Net: "tcp",
		Dialer: &net.Dialer{
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					// FIXME: remove
					terminal.Errorf("agent.DialerFromContext()\n")
					dialer := agent.DialerFromContext(ctx)
					if dialer == nil {
						return nil, fmt.Errorf("failed to get dialer from context")
					}
					// FIXME: remove
					terminal.Errorf("dialer.DialContext(ctx, network, address) network: %s address: %s\n", network, address)
					return dialer.DialContext(ctx, network, address)
				},
			},
		},
	}
	return newDnsClient(dnsClient, client.FromContext(ctx))
}

func newDnsClient(dnsClient *dns.Client, flyClient *client.Client) *DnsClient {
	return &DnsClient{
		dnsClient: dnsClient,
		flyClient: flyClient,
	}
}

// queries 'name IN AAAA' and returns ordered slice of net.IPs
func (dc *DnsClient) LookupAAAA(ctx context.Context, name string) ([]net.IP, error) {
	if !strings.HasSuffix(name, ".") {
		name = name + "."
	}
	// appName, err := appNameFromQuery(name)
	// if err != nil {
	// 	return nil, err
	// }
	// FIXME: implement rest of this
	return nil, fmt.Errorf("not implemented yet")
}

func (dc *DnsClient) LookupAppAAAA(ctx context.Context, appName string) ([]net.IP, error) {
	// FIXME: do these queries concurrently (tvd, 2022-10-20)
	gqlResult, err := dc.gql6pnFromAppName(ctx, appName)
	if err != nil {
		return nil, err
	}
	// dnsAnswer, err := dc.dnsQuery(ctx, fmt.Sprintf("%s.internal.", appName), dns.TypeAAAA)
	// dnsResult := make([]net.IP, 0)
	// for _, answer := range dnsAnswer {
	// 	if aaaa, ok := answer.(*dns.AAAA); ok {
	// 		dnsResult = append(dnsResult, aaaa.AAAA)
	// 	}
	// }
	// FIXME: verify gql and dns results match, and if not report error to Fly error service
	return gqlResult, err
}

// queries 'name IN TXT' and returns ordered slice of strings
func (dc *DnsClient) LookupTXT(name string) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func appNameFromQuery(name string) (string, error) {
	// add trailing . if missing
	if !strings.HasSuffix(name, ".internal.") {
		return "", fmt.Errorf("only .internal names are supported, not %s", name)
	}
	parts := strings.Split(name, ".internal.")
	appName := parts[len(parts)-2]
	return appName, nil
}

const internalResolver string = "[fdaa::3]:53"

func (dc *DnsClient) dnsQuery(ctx context.Context, name string, qType uint16) ([]dns.RR, error) {
	// FIXME: send query through agent (maybe use client get instances() call)
	return nil, fmt.Errorf("not implemented")
}

func (dc *DnsClient) gql6pnFromAppName(ctx context.Context, appName string) ([]net.IP, error) {
	gqlClient := dc.flyClient.API().GenqClient
	_ = `# @genqlient
	query InternalDnsGetAllocs($appName: String!) {
		app(name: $appName) {
			id
			name
			allocations(showCompleted: false) {
				id
				idShort
				region
				privateIP
			}
			machines {
				nodes {
					id
					region
					ips {
						nodes {
							kind
							family
							ip
						}
					}
				}
			}
		}
	}
	`
	resp, err := gql.InternalDnsGetAllocs(ctx, gqlClient, appName)
	if err != nil {
		return nil, err
	}
	result := make([]net.IP, 0)
	// FIXME: deal with machine ips when they are returned...
	for _, alloc := range resp.App.Allocations {
		ip := net.ParseIP(alloc.PrivateIP)
		if ip == nil {
			// FIXME: report this error to sentry???
		} else {
			result = append(result, ip)
		}
	}
	return result, nil
}
