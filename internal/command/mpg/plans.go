package mpg

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/logger"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// PlanDetails holds the details for each managed postgres plan.
type PlanDetails struct {
	Name       string
	CPU        string
	Memory     string
	PricePerMo int
}

var MPGPlans = map[string]PlanDetails{
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
		Memory:     "33 GB",
		PricePerMo: 962,
	},
	"Performance": {
		Name:       "Performance",
		CPU:        "Performance x 8",
		Memory:     "64 GB",
		PricePerMo: 1922,
	},
}

// orgPlans returns the plans the API offers orgSlug, keyed by slug like
// MPGPlans. Plans MPGPlans knows keep its details; any other, such as dev-256,
// which only some orgs are offered, is described from the API's fields. If
// listing fails or finds none, MPGPlans stands in.
func orgPlans(ctx context.Context, orgSlug string) map[string]PlanDetails {
	listed, err := flapsutil.ClientFromContext(ctx).ListManagedPostgresPlans(ctx, orgSlug)
	if err == nil && len(listed) == 0 {
		err = errors.New("no plans listed")
	}
	if err != nil {
		logger.MaybeFromContext(ctx).Debugf("using the built-in Managed Postgres plans for %s: %v", orgSlug, err)
		return MPGPlans
	}

	plans := make(map[string]PlanDetails, len(listed))
	for _, p := range listed {
		if known, ok := MPGPlans[p.Plan]; ok {
			plans[p.Plan] = known
			continue
		}

		memory := fmt.Sprintf("%d MB", p.MemoryMB)
		if p.MemoryMB%1024 == 0 {
			memory = fmt.Sprintf("%d GB", p.MemoryMB/1024)
		}

		plans[p.Plan] = PlanDetails{
			Name:       p.Plan,
			CPU:        fmt.Sprintf("%s x %d", cases.Title(language.English).String(p.CPUKind), p.CPUs),
			Memory:     memory,
			PricePerMo: p.MonthlyPriceUSD,
		}
	}

	return plans
}
