package ips

import (
	"context"
	"net"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func renderListTable(ctx context.Context, ipAddresses []fly.IPAddress) {
	rows := make([][]string, 0, len(ipAddresses))

	var ipType string
	for _, ipAddr := range ipAddresses {
		if strings.HasPrefix(ipAddr.Address, "fdaa") {
			ipType = "private"
		} else {
			ipType = "public"
		}

		createdAt := format.RelativeTime(ipAddr.CreatedAt)

		switch {
		case ipAddr.Type == "v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "public (dedicated, $2/mo)", ipAddr.Region, "", ipAddr.ServiceName, createdAt})
		case ipAddr.Type == "shared_v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "public (shared)", ipAddr.Region, "", ipAddr.ServiceName, createdAt})
		case ipAddr.Type == "v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "public (dedicated)", ipAddr.Region, formatNetwork(ipAddr), ipAddr.ServiceName, createdAt})
		case ipAddr.Type == "private_v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "private", ipAddr.Region, formatNetwork(ipAddr), ipAddr.ServiceName, createdAt})
		default:
			rows = append(rows, []string{ipAddr.Type, ipAddr.Address, ipType, ipAddr.Region, "", ipAddr.ServiceName, createdAt})
		}
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Version", "IP", "Type", "Region", "Network", "Service", "Created At")
}

func formatNetwork(ipAddr fly.IPAddress) string {
	if ipAddr.Network == nil {
		return ""
	}
	name := ipAddr.Network.Name
	if name == "" {
		name = "default"
	}

	if ipAddr.Network.Organization != nil {
		return ipAddr.Network.Organization.Slug + "/" + name
	}

	return name
}

func renderPrivateTableMachines(ctx context.Context, machines []*fly.Machine) {
	rows := make([][]string, 0, len(machines))

	for _, machine := range machines {
		rows = append(rows, []string{machine.ID, machine.Region, machine.PrivateIP})
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "ID", "Region", "IP")
}

func renderSharedTable(ctx context.Context, ip net.IP) {
	rows := make([][]string, 0, 1)

	rows = append(rows, []string{"v4", ip.String(), "shared", "global"})

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Version", "IP", "Type", "Region")
}
