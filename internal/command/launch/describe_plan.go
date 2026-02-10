package launch

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/command/mpg"
	"github.com/superfly/flyctl/internal/command/redis"
)

// This had to be broken out from `plan` because of a circular dependency.
// Easily the most annoying part of go.

const descriptionNone = "<none>"

func describePostgresPlan(launchPlan *plan.LaunchPlan) (string, error) {
	switch provider := launchPlan.Postgres.Provider().(type) {
	case *plan.FlyPostgresPlan:
		return describeFlyPostgresPlan(provider)
	case *plan.SupabasePostgresPlan:
		return describeSupabasePostgresPlan(provider, launchPlan)
	case *plan.ManagedPostgresPlan:
		return describeManagedPostgresPlan(provider, launchPlan)
	}
	return descriptionNone, nil
}

func describeFlyPostgresPlan(p *plan.FlyPostgresPlan) (string, error) {
	guestStr := ""
	if p.VmRam > 1024 {
		guestStr = fmt.Sprintf("%s, %dGB RAM", p.VmSize, p.VmRam/1024)
	} else {
		guestStr = fmt.Sprintf("%s, %dMB RAM", p.VmSize, p.VmRam)
	}
	diskSizeStr := fmt.Sprintf("%dGB disk", p.DiskSizeGB)

	info := []string{guestStr, diskSizeStr}
	if p.AutoStop {
		info = append(info, "auto-stop")
	}
	if p.Price > 0 {
		info = append(info, fmt.Sprintf("$%d/mo", p.Price))
	}

	return strings.Join(info, ", "), nil
}

func describeSupabasePostgresPlan(p *plan.SupabasePostgresPlan, launchPlan *plan.LaunchPlan) (string, error) {

	return fmt.Sprintf("(Supabase) %s in %s", p.GetDbName(launchPlan), p.GetRegion(launchPlan)), nil
}

func describeRedisPlan(ctx context.Context, p plan.RedisPlan) (string, error) {

	switch provider := p.Provider().(type) {
	case *plan.UpstashRedisPlan:
		return describeUpstashRedisPlan(ctx, provider)
	}
	return descriptionNone, nil
}

func describeUpstashRedisPlan(ctx context.Context, p *plan.UpstashRedisPlan) (string, error) {
	plan, err := redis.DeterminePlan(ctx, "")
	if err != nil {
		return "<plan not found, this is an error>", fmt.Errorf("redis plan not found: %w", err)
	}

	evictionStatus := lo.Ternary(p.Eviction, "enabled", "disabled")
	return fmt.Sprintf("%s Plan: %s Max Data Size, eviction %s", plan.DisplayName, plan.MaxDataSize, evictionStatus), nil
}

func describeObjectStoragePlan(p plan.ObjectStoragePlan) (string, error) {
	if p.TigrisObjectStorage == nil {
		return descriptionNone, nil
	}

	return "private bucket", nil
}

func describeManagedPostgresPlan(p *plan.ManagedPostgresPlan, launchPlan *plan.LaunchPlan) (string, error) {
	info := []string{}

	planDetails, ok := mpg.MPGPlans[p.Plan]

	if p.DbName != "" {
		info = append(info, fmt.Sprintf("\"%s\"", p.GetDbName(launchPlan)))
	}

	if ok {
		info = append(info, fmt.Sprintf("%s plan ($%d/mo)", planDetails.Name, planDetails.PricePerMo))
	} else {
		info = append(info, fmt.Sprintf("plan %s", p.Plan))
	}

	if p.Region != "" {
		info = append(info, fmt.Sprintf("region %s", p.GetRegion(launchPlan)))
	}

	return strings.Join(info, ", "), nil
}
