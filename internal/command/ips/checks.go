package ips

import (
	"context"
	"fmt"
	"math"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

const MAX_MACHINES_PER_APP_EGRESS = 64

type appScopedEgressIpsRegionCounters struct {
	v4 int
	v6 int
}

func SanityCheckAppScopedEgressIps(ctx context.Context, regionFilter map[string]any, ips map[string][]fly.EgressIPAddress, machines []*fly.Machine) {
	var err error

	client := flyutil.ClientFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)
	errOut := iostreams.FromContext(ctx).ErrOut
	appName := appconfig.NameFromContext(ctx)

	if ips == nil {
		ips, err = client.GetAppScopedEgressIPAddresses(ctx, appName)
		if err != nil {
			return
		}
	}

	ipRegions := make(map[string]appScopedEgressIpsRegionCounters)
	machineRegions := make(map[string]int)
	hasEgressIps := false

	for region, regionalIps := range ips {
		var counter appScopedEgressIpsRegionCounters
		for _, ip := range regionalIps {
			switch ip.Version {
			case 4:
				counter.v4 += 1
			case 6:
				counter.v6 += 1
			}
		}

		if counter.v4 != 0 || counter.v6 != 0 {
			hasEgressIps = true
		}

		ipRegions[region] = counter
	}

	// Short-circuit
	if !hasEgressIps {
		return
	}

	if machines == nil {
		machines, err = flapsClient.List(ctx, appName, "")
		if err != nil {
			return
		}
	}

	for _, m := range machines {
		if _, ok := machineRegions[m.Region]; !ok {
			machineRegions[m.Region] = 0
		}

		machineRegions[m.Region] += 1
	}

	for region, ipCounter := range ipRegions {
		// Only apply the filter before we emit warnings -- since we might need to know whether this apps has egress IPs anywhere
		if _, ok := regionFilter[region]; regionFilter != nil && !ok {
			continue
		}

		machineCount, ok := machineRegions[region]
		if !ok || machineCount == 0 {
			fmt.Fprintf(errOut, "Warning: Your app has egress IP(s) assigned in region %s but you have no machines there.\n", region)
		}

		if ipCounter.v4 != ipCounter.v6 {
			if ipCounter.v4 == 0 {
				fmt.Fprintf(errOut, "Warning: Your app has egress IPv6 assigned in region %s but no IPv4.\n", region)
			} else if ipCounter.v6 == 0 {
				fmt.Fprintf(errOut, "Warning: Your app has egress IPv4 assigned in region %s but no IPv6.\n", region)
			} else {
				fmt.Fprintf(errOut, "Warning: Your app has a different number of egress IPv4 (%d) and IPv6 (%d) in region %s. If this is not intentional, please release some excess IPs as you will be billed for unused egress IPs as well.\n", ipCounter.v4, ipCounter.v6, region)
			}
		}

		if machineCount >= MAX_MACHINES_PER_APP_EGRESS*ipCounter.v4 {
			fmt.Fprintf(errOut, "Warning: Your app has %d machines in region %s but only %d egress IPv4(s). You need at least %d more to cover all machines.\n", machineCount, region, ipCounter.v4, int(math.Ceil(float64(machineCount)/float64(MAX_MACHINES_PER_APP_EGRESS)))-ipCounter.v4)
		}

		if machineCount >= MAX_MACHINES_PER_APP_EGRESS*ipCounter.v6 {
			fmt.Fprintf(errOut, "Warning: Your app has %d machines in region %s but only %d egress IPv6(s). You need at least %d more to cover all machines.\n", machineCount, region, ipCounter.v6, int(math.Ceil(float64(machineCount)/float64(MAX_MACHINES_PER_APP_EGRESS)))-ipCounter.v6)
		}
	}

	for region := range machineRegions {
		if _, ok := regionFilter[region]; regionFilter != nil && !ok {
			continue
		}

		ipCounter, ok := ipRegions[region]
		if hasEgressIps && (!ok || (ipCounter.v4 == 0 && ipCounter.v6 == 0)) {
			fmt.Fprintf(errOut, "Warning: Your app has machines in region %s but no egress IPs allocated there. Since you have egress IPs assigned elsewhere, you may want to assign an egress IP in this region as well.\n", region)
			continue
		}
	}
}
