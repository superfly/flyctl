package app

import (
	"math"

	"github.com/superfly/flyctl/api"
)

type HTTPService struct {
	InternalPort int                            `json:"internal_port" toml:"internal_port" validate:"required,numeric"`
	ForceHttps   bool                           `toml:"force_https"`
	Concurrency  *api.MachineServiceConcurrency `toml:"concurrency,omitempty"`
}

func (svc *HTTPService) toMachineService() *api.MachineService {
	concurrency := svc.Concurrency
	if concurrency != nil {
		if concurrency.Type == "" {
			concurrency.Type = "requests"
		}
		if concurrency.HardLimit == 0 {
			concurrency.HardLimit = 25
		}
		if concurrency.SoftLimit == 0 {
			concurrency.SoftLimit = int(math.Ceil(float64(concurrency.HardLimit) * 0.8))
		}
	}
	return &api.MachineService{
		Protocol:     "tcp",
		InternalPort: svc.InternalPort,
		Ports: []api.MachinePort{{
			Port:       api.IntPointer(80),
			Handlers:   []string{"http"},
			ForceHttps: svc.ForceHttps,
		}, {
			Port:     api.IntPointer(443),
			Handlers: []string{"http", "tls"},
		}},
		Concurrency: concurrency,
	}
}
