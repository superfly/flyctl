package ips

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		long  = `Lists the IP addresses allocated to the application`
		short = `List allocated IP addresses`
	)

	cmd := command.New("list", short, long, runIPAddressesList,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
	)

	return cmd
}

func runIPAddressesList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	appName := appconfig.NameFromContext(ctx)

	res, err := flapsClient.GetIPAssignments(ctx, appName)
	if err != nil {
		return err
	}

	ipAddresses := ipAssignmentsToIPAddresses(res.IPs)

	if cfg.JSONOutput {
		return render.JSON(out, ipAddresses)
	}

	renderListTable(ctx, ipAddresses)
	SanityCheckAppScopedEgressIps(ctx, nil, egressIPAddressesByRegion(res.IPs), nil, "")
	fmt.Println("Learn more about Fly.io public, private, shared and dedicated IP addresses in our docs: https://fly.io/docs/networking/services/")

	return nil
}

// ipAssignmentsToIPAddresses converts Machines API IP assignments into the fly.IPAddress shape
// used by the table renderer and the JSON output, so the output format stays stable.
func ipAssignmentsToIPAddresses(assignments []flaps.IPAssignment) []fly.IPAddress {
	ipAddresses := make([]fly.IPAddress, 0, len(assignments))
	for _, ip := range assignments {
		ipAddresses = append(ipAddresses, fly.IPAddress{
			Address:     ip.IP,
			Type:        string(ip.Type()),
			Region:      ip.Region,
			CreatedAt:   ip.CreatedAt,
			ServiceName: ip.ServiceName,
		})
	}
	return ipAddresses
}

// egressIPAddressesByRegion picks the egress IPs out of a list of assignments and groups them by
// region, in the shape expected by SanityCheckAppScopedEgressIps.
func egressIPAddressesByRegion(assignments []flaps.IPAssignment) map[string][]fly.EgressIPAddress {
	egressIPs := make(map[string][]fly.EgressIPAddress)
	for _, ip := range assignments {
		if !ip.Egress {
			continue
		}
		version := 4
		if ip.IsV6() {
			version = 6
		}
		egressIPs[ip.Region] = append(egressIPs[ip.Region], fly.EgressIPAddress{
			IP:        ip.IP,
			Version:   version,
			Region:    ip.Region,
			UpdatedAt: ip.CreatedAt,
		})
	}
	return egressIPs
}
