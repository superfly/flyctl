package dnsclient

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
)

// Internal DnsClient, which queries both the graphql api and .internal dns to answer queries
// Graphql wins when there are differences, and those discrepancies are reported to Fly's error recording system
type DnsClient struct {
	dnsClient *dns.Client
	flyClient *client.Client
}

func DnsClientFromContext(ctx context.Context) *DnsClient {
	return NewDnsClient(&dns.Client{}, client.FromContext(ctx))
}

func NewDnsClient(dnsClient *dns.Client, flyClient *client.Client) *DnsClient {
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
	dnsAnswer, err := dc.dnsQuery(ctx, fmt.Sprintf("%s.internal.", appName), dns.TypeAAAA)
	dnsResult := make([]net.IP, 0)
	for _, answer := range dnsAnswer {
		if aaaa, ok := answer.(*dns.AAAA); ok {
			dnsResult = append(dnsResult, aaaa.AAAA)
		}
	}
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

func (dc *DnsClient) dnsQuery(ctx context.Context, name string, qType uint16) ([]dns.RR, error) {
	msg := &dns.Msg{}
	msg.SetQuestion(name, qType)
	msg.RecursionDesired = false
	resp, _, err := dc.dnsClient.Exchange(msg, "fdaa::3")
	if err != nil {
		return nil, err
	}
	if resp.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("invalid result when querying %s record for %s: %s", dns.TypeToString[qType], name, dns.RcodeToString[resp.Rcode])
	}
	return resp.Answer, nil
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
