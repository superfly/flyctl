package launch

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/command/redis"
)

// This had to be broken out from `plan` because of a circular dependency.
// Easily the most annoying part of go.

const descriptionNone = "<none>"

func describePostgresPlan(ctx context.Context, p plan.PostgresPlan, org *api.Organization) (string, error) {
	provider := p.Provider()
	switch provider.(type) {
	case plan.FlyPostgresPlan:
		return describeFlyPostgresPlan(ctx, provider.(*plan.FlyPostgresPlan), org)
	}
	return descriptionNone, nil
}

func describeFlyPostgresPlan(ctx context.Context, p *plan.FlyPostgresPlan, org *api.Organization) (string, error) {

	nodePlural := lo.Ternary(p.Nodes == 1, "", "s")
	nodesStr := fmt.Sprintf("%d Node%s", p.Nodes, nodePlural)

	guestStr := p.VmSize
	if p.VmRam > 0 {
		guest := api.MachinePresets[p.VmSize]
		if guest.MemoryMB != p.VmRam {
			guestStr = fmt.Sprintf("%s (%dGB RAM)", guest, p.VmRam/1024)
		}
	}

	diskSizeStr := fmt.Sprintf("%dGB disk", p.DiskSizeGB)

	info := []string{nodesStr, guestStr, diskSizeStr}
	if p.AutoStop {
		info = append(info, "auto-stop")
	}

	return strings.Join(info, ", "), nil
}

func describeRedisPlan(ctx context.Context, p plan.RedisPlan, org *api.Organization) (string, error) {
	provider := p.Provider()
	switch provider.(type) {
	case plan.UpstashRedisPlan:
		return describeUpstashRedisPlan(ctx, provider.(*plan.UpstashRedisPlan), org)
	}
	return descriptionNone, nil
}

func describeUpstashRedisPlan(ctx context.Context, p *plan.UpstashRedisPlan, org *api.Organization) (string, error) {

	plan, err := redis.DeterminePlan(ctx, org)
	if err != nil {
		return "<plan not found, this is an error>", fmt.Errorf("redis plan not found: %w", err)
	}

	evictionStatus := lo.Ternary(p.Eviction, "enabled", "disabled")
	return fmt.Sprintf("%s Plan: %s Max Data Size, eviction %s", plan.DisplayName, plan.MaxDataSize, evictionStatus), nil
}
