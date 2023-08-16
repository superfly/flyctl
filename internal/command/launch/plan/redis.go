package plan

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/terminal"
)

type redisPlan struct {
	UpstashRedis *upstashRedisPlan `json:"upstash_redis" url:"upstash_redis"`
}

func (p *redisPlan) Describe(ctx context.Context) (string, error) {
	if p == nil {
		return descriptionNone, nil
	}
	if p.UpstashRedis != nil {
		return p.UpstashRedis.Describe(ctx)
	}
	return descriptionNone, nil
}

type upstashRedisPlan struct {
	AppName      string   `json:"app_name" url:"app_name"`
	PlanId       string   `json:"plan_id" url:"plan_id"`
	Eviction     bool     `json:"eviction" url:"eviction"`
	ReadReplicas []string `json:"read_replicas" url:"read_replicas"`
}

func (p *upstashRedisPlan) Describe(ctx context.Context) (string, error) {

	apiClient := client.FromContext(ctx).API()

	result, err := gql.ListAddOnPlans(ctx, apiClient.GenqClient)
	if err != nil {
		terminal.Debugf("Failed to list addon plans: %s\n", err)
		return "<internal error>", err
	}

	for _, plan := range result.AddOnPlans.Nodes {
		if plan.Id == p.PlanId {
			evictionStatus := lo.Ternary(p.Eviction, "enabled", "disabled")
			return fmt.Sprintf("%s: %s Max Data Size, ($%d / month), eviction %s", plan.DisplayName, plan.MaxDataSize, plan.PricePerMonth, evictionStatus), nil
		}
	}

	return "<plan not found, this is an error>", fmt.Errorf("plan not found: %s", p.PlanId)
}
