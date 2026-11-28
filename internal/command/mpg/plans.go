package mpg

import (
	"context"
	"fmt"
	"sort"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
)

// availablePlans returns known plans in local price order. A failed lookup
// returns the full local catalog and the error so callers can explain the fallback.
func availablePlans(ctx context.Context, orgSlug string) ([]flaps.ManagedPostgresPlan, error) {
	plans, err := flapsutil.ClientFromContext(ctx).ListManagedPostgresPlans(ctx, orgSlug)
	if err != nil {
		plans = make([]flaps.ManagedPostgresPlan, 0, len(MPGPlans))
		for key := range MPGPlans {
			plans = append(plans, flaps.ManagedPostgresPlan{Plan: key})
		}
	}
	if len(plans) == 0 {
		return nil, fmt.Errorf("no Managed Postgres plans available for organization %s", orgSlug)
	}
	known := make([]flaps.ManagedPostgresPlan, 0, len(plans))
	for _, p := range plans {
		if _, ok := MPGPlans[p.Plan]; ok {
			known = append(known, p)
		}
	}
	plans = known
	if len(plans) == 0 {
		return nil, fmt.Errorf("Managed Postgres plans for organization %s are not supported by this flyctl version; upgrade flyctl", orgSlug)
	}

	sort.Slice(plans, func(i, j int) bool { return MPGPlans[plans[i].Plan].PricePerMo < MPGPlans[plans[j].Plan].PricePerMo })

	return plans, err
}

// defaultPlan prefers Basic and otherwise picks the cheapest available plan.
// plans must be nonempty and in local price order, as returned by availablePlans.
func defaultPlan(plans []flaps.ManagedPostgresPlan) string {
	for _, p := range plans {
		if p.Plan == "basic" {
			return p.Plan
		}
	}

	return plans[0].Plan
}

// PlanDetails holds the details for each managed postgres plan.
type PlanDetails struct {
	Name       string
	CPU        string
	Memory     string
	PricePerMo int
}

var MPGPlans = map[string]PlanDetails{
	"dev-256": {
		Name:       "Developer",
		CPU:        "Shared x 1",
		Memory:     "256 MB",
		PricePerMo: 4,
	},
	"basic": {
		Name:       "Basic",
		CPU:        "Shared x 2",
		Memory:     "1 GB",
		PricePerMo: 38,
	},
	"starter": {
		Name:       "Starter",
		CPU:        "Shared x 2",
		Memory:     "2 GB",
		PricePerMo: 72,
	},
	"launch": {
		Name:       "Launch",
		CPU:        "Performance x 2",
		Memory:     "8 GB",
		PricePerMo: 282,
	},
	"scale": {
		Name:       "Scale",
		CPU:        "Performance x 4",
		Memory:     "32 GB",
		PricePerMo: 962,
	},
	"Performance": {
		Name:       "Performance",
		CPU:        "Performance x 8",
		Memory:     "64 GB",
		PricePerMo: 1922,
	},
}
