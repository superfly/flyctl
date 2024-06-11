package launch

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command/launch/plan"
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
	}
	return descriptionNone, nil
}

func describeFlyPostgresPlan(p *plan.FlyPostgresPlan) (string, error) {

	nodePlural := lo.Ternary(p.Nodes == 1, "", "s")
	nodesStr := fmt.Sprintf("(Fly Postgres) %d Node%s", p.Nodes, nodePlural)

	guestStr := fly.MachinePresets[p.VmSize].String()

	diskSizeStr := fmt.Sprintf("%dGB disk", p.DiskSizeGB)

	info := []string{nodesStr, guestStr, diskSizeStr}
	if p.AutoStop {
		info = append(info, "auto-stop")
	}

	return strings.Join(info, ", "), nil
}

func describeSupabasePostgresPlan(p *plan.SupabasePostgresPlan, launchPlan *plan.LaunchPlan) (string, error) {

	return fmt.Sprintf("(Supabase) %s in %s", p.GetDbName(launchPlan), p.GetRegion(launchPlan)), nil
}

func describeRedisPlan(ctx context.Context, p plan.RedisPlan, org *fly.Organization) (string, error) {

	switch provider := p.Provider().(type) {
	case *plan.UpstashRedisPlan:
		return describeUpstashRedisPlan(ctx, provider, org)
	}
	return descriptionNone, nil
}

func describeUpstashRedisPlan(ctx context.Context, p *plan.UpstashRedisPlan, org *fly.Organization) (string, error) {

	plan, err := redis.DeterminePlan(ctx, org)
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
