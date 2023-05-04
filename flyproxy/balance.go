package flyproxy

type BalanceResponse struct {
	Total    uint                `json:"total"`
	Chosen   *BalancedInstance   `json:"chosen"`
	Rejected []*BalancedInstance `json:"rejected"`
}

type BalancedInstance struct {
	ID          string            `json:"id"`
	Region      string            `json:"region"`
	State       string            `json:"state"`
	Concurrency int               `json:"concurrency"`
	Healthy     bool              `json:"healthy"`
	NodeHealthy bool              `json:"node_healthy"`
	NodeRttMs   float64           `json:"node_rtt_ms"`
	Rejection   *BalanceRejection `json:"rejection"`
}

type BalanceRejection struct {
	ID   string `json:"id"`
	Desc string `json:"description"`
}
