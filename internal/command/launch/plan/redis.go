package plan

type RedisPlan struct {
	UpstashRedis *UpstashRedisPlan `json:"upstash_redis"`
}

func (p *RedisPlan) Provider() any {
	if p == nil {
		return nil
	}
	if p.UpstashRedis != nil {
		return p.UpstashRedis
	}
	return nil
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
