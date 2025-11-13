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

func renderListTable(ctx context.Context, ipAddresses []flaps.IPAssignment) {
	rows := make([][]string, 0, len(ipAddresses))

	for _, ipAddr := range ipAddresses {
		createdAt := format.RelativeTime(ipAddr.CreatedAt)

		switch {
		case ipAddr.Shared:
			rows = append(rows, []string{"v4", ipAddr.IP, "public (shared)", ipAddr.Region, createdAt})
		case ipAddr.IsFlycast():
			rows = append(rows, []string{"v6", ipAddr.IP, "private", ipAddr.Region, createdAt})
		case strings.Contains(ipAddr.IP, "."):
			rows = append(rows, []string{"v4", ipAddr.IP, "public (dedicated, $2/mo)", ipAddr.Region, createdAt})
		case strings.Contains(ipAddr.IP, ":"):
			rows = append(rows, []string{"v6", ipAddr.IP, "public (dedicated)", ipAddr.Region, createdAt})
		default:
			rows = append(rows, []string{"unknown", ipAddr.IP, "unknown", ipAddr.Region, createdAt})
		}
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Version", "IP", "Type", "Region", "Created At")
}

func renderPrivateTableMachines(ctx context.Context, machines []*fly.Machine) {
	rows := make([][]string, 0, len(machines))

	for _, machine := range machines {
		rows = append(rows, []string{machine.ID, machine.Region, machine.PrivateIP})
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "ID", "Region", "IP")
}

func renderSharedTable(ctx context.Context, ip flaps.IPAssignment) {
	rows := make([][]string, 0, 1)

	rows = append(rows, []string{"v4", ip.IP, "shared", "global"})

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Version", "IP", "Type", "Region")
}
