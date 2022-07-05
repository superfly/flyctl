package ips

import (
	"context"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func renderListTable(ctx context.Context, ipAddresses []api.IPAddress) {
	rows := make([][]string, 0, len(ipAddresses))

	var ipType string
	for _, ipAddr := range ipAddresses {
		if strings.HasPrefix(ipAddr.Address, "fdaa") {
			ipType = "private"
		} else {
			ipType = "public"
		}

		rows = append(rows, []string{ipAddr.Type, ipAddr.Address, ipType, ipAddr.Region, presenters.FormatRelativeTime(ipAddr.CreatedAt)})
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "Version", "IP", "Type", "Region", "Created At")
}

func renderPrivateTable(ctx context.Context, allocations []*api.AllocationStatus, backupRegions []api.Region) {
	rows := make([][]string, 0, len(allocations))

	for _, alloc := range allocations {

		region := alloc.Region
		if len(backupRegions) > 0 {
			for _, r := range backupRegions {
				if alloc.Region == r.Code {
					region = alloc.Region + "(B)"
					break
				}
			}
		}

		rows = append(rows, []string{alloc.IDShort, region, alloc.PrivateIP})
	}

	out := iostreams.FromContext(ctx).Out
	render.Table(out, "", rows, "ID", "Region", "IP")
}
