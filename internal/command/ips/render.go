package ips

import (
	"context"
	"slices"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func renderListTable(ctx context.Context, appName string, ipAddresses []fly.IPAddress) {
	rows := make([][]string, 0, len(ipAddresses))
	appOrgSlug := appOrgSlugForNetworks(ctx, appName, ipAddresses)

	var ipType string
	for _, ipAddr := range ipAddresses {
		if strings.HasPrefix(ipAddr.Address, "fdaa") {
			ipType = "private"
		} else {
			ipType = "public"
		}

		createdAt := format.RelativeTime(ipAddr.CreatedAt)
		network := networkName(ipAddr, appOrgSlug)

		switch ipAddr.Type {
		case "v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "public ingress (dedicated, $2/mo)", ipAddr.Region, network, createdAt})
		case "shared_v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "public ingress (shared)", ipAddr.Region, network, createdAt})
		case "v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "public ingress (dedicated)", ipAddr.Region, network, createdAt})
		case "private_v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "private ingress", ipAddr.Region, network, createdAt})
		case "egress_v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "egress", ipAddr.Region, network, createdAt})
		case "egress_v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "egress", ipAddr.Region, network, createdAt})
		default:
			rows = append(rows, []string{ipAddr.Type, ipAddr.Address, ipType, ipAddr.Region, network, createdAt})
		}
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Version", "IP", "Type", "Region", "Network", "Created At")
}

// appOrgSlugForNetworks returns the app's org slug, used to hide the org prefix on networks
// belonging to the app's own org. It is only fetched when some address has a network; "" is
// returned on failure, in which case the org prefix is shown for every network.
func appOrgSlugForNetworks(ctx context.Context, appName string, ipAddresses []fly.IPAddress) string {
	if !slices.ContainsFunc(ipAddresses, func(ip fly.IPAddress) bool { return ip.Network != nil }) {
		return ""
	}

	app, err := flapsutil.ClientFromContext(ctx).GetApp(ctx, appName)
	if err != nil {
		return ""
	}

	return app.Organization.Slug
}

// networkName returns the 6PN network a Flycast address belongs to, or "" for other IP types.
// The organization's default network has an empty name and is shown as "default". Networks in
// an org other than the app's own are shown as "org-slug/name", which is unambiguous: network
// names ([a-z][a-z0-9-]*) and org slugs cannot contain "/".
func networkName(ipAddr fly.IPAddress, appOrgSlug string) string {
	if ipAddr.Network == nil {
		return ""
	}

	name := ipAddr.Network.Name
	if name == "" {
		name = "default"
	}
	if ipAddr.Network.Organization == nil || ipAddr.Network.Organization.Slug == appOrgSlug {
		return name
	}

	return ipAddr.Network.Organization.Slug + "/" + name
}

// renderAssignedIP renders a single newly-assigned (non egress-pair) IP address.
func renderAssignedIP(ctx context.Context, appName string, res *flaps.AssignIPResponse) {
	if res.IP == nil {
		return
	}

	renderListTable(ctx, appName, ipAssignmentsToIPAddresses([]flaps.IPAssignment{{
		IP:          *res.IP,
		Region:      res.Region,
		ServiceName: res.ServiceName,
		Shared:      res.Shared,
		CreatedAt:   res.CreatedAt,
		Egress:      res.Egress,
		Network:     res.Network,
	}}))
}

func renderPrivateTableMachines(ctx context.Context, machines []*fly.Machine) {
	rows := make([][]string, 0, len(machines))

	for _, machine := range machines {
		rows = append(rows, []string{machine.ID, machine.Region, machine.PrivateIP})
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "ID", "Region", "IP")
}
