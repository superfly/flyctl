package plan

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/command/redis"
)

type RedisProvider interface {
	Describe(ctx context.Context, org *api.Organization) (string, error)
}

type RedisPlan struct {
	UpstashRedis *UpstashRedisPlan `json:"upstash_redis"`
}

func (p *RedisPlan) Provider() RedisProvider {
	if p == nil {
		return nil
	}
	if p.UpstashRedis != nil {
		return p.UpstashRedis
	}
	return nil
}

func (p *RedisPlan) Describe(ctx context.Context, org *api.Organization) (string, error) {
	if provider := p.Provider(); provider != nil {
		return provider.Describe(ctx, org)
	}
	return descriptionNone, nil
}

func DefaultRedis(plan *LaunchPlan) RedisPlan {
	return RedisPlan{
		UpstashRedis: &UpstashRedisPlan{
			Eviction: false,
		},
	}
}

type UpstashRedisPlan struct {
	Eviction     bool     `json:"eviction"`
	ReadReplicas []string `json:"read_replicas"`
}

func (p *UpstashRedisPlan) Describe(ctx context.Context, org *api.Organization) (string, error) {

	plan, err := redis.DeterminePlan(ctx, org)
	if err != nil {
		return "<plan not found, this is an error>", fmt.Errorf("redis plan not found: %w", err)
	}

	evictionStatus := lo.Ternary(p.Eviction, "enabled", "disabled")
	return fmt.Sprintf("%s Plan: %s Max Data Size, eviction %s", plan.DisplayName, plan.MaxDataSize, evictionStatus), nil
}
