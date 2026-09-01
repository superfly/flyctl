package ips

import (
	"context"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
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

		switch ipAddr.Type {
		case "v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "public ingress (dedicated, $2/mo)", ipAddr.Region, createdAt})
		case "shared_v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "public ingress (shared)", ipAddr.Region, createdAt})
		case "v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "public ingress (dedicated)", ipAddr.Region, createdAt})
		case "private_v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "private ingress", ipAddr.Region, createdAt})
		case "egress_v4":
			rows = append(rows, []string{"v4", ipAddr.Address, "egress", ipAddr.Region, createdAt})
		case "egress_v6":
			rows = append(rows, []string{"v6", ipAddr.Address, "egress", ipAddr.Region, createdAt})
		default:
			rows = append(rows, []string{ipAddr.Type, ipAddr.Address, ipType, ipAddr.Region, createdAt})
		}
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Version", "IP", "Type", "Region", "Created At")
}

// renderAssignedIP renders a single newly-assigned (non egress-pair) IP address.
func renderAssignedIP(ctx context.Context, res *flaps.AssignIPResponse) {
	if res.IP == nil {
		return
	}

	renderListTable(ctx, ipAssignmentsToIPAddresses([]flaps.IPAssignment{{
		IP:          *res.IP,
		Region:      res.Region,
		ServiceName: res.ServiceName,
		Shared:      res.Shared,
		CreatedAt:   res.CreatedAt,
		Egress:      res.Egress,
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
