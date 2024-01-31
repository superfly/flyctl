package plan

import (
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/version"
)

type LaunchPlan struct {
	AppName string `json:"name"`
	OrgSlug string `json:"org"`

	RegionCode       string `json:"region"`
	HighAvailability bool   `json:"ha"`

	Compute []*appconfig.Compute `json:"compute"`

	HttpServicePort int `json:"http_service_port,omitempty"`

	Postgres PostgresPlan `json:"postgres"`

	Redis RedisPlan `json:"redis"`

	ScannerFamily string          `json:"scanner_family"`
	FlyctlVersion version.Version `json:"flyctl_version"`
}
