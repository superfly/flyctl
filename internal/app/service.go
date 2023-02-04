package app

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
)

type Service struct {
	Protocol     string                         `json:"protocol,omitempty" toml:"protocol"`
	InternalPort int                            `json:"internal_port,omitempty" toml:"internal_port"`
	Ports        []api.MachinePort              `json:"ports,omitempty" toml:"ports"`
	Concurrency  *api.MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
	TCPChecks    []*ServiceTCPCheck             `json:"tcp_checks,omitempty" toml:"tcp_checks,omitempty"`
	HTTPChecks   []*ServiceHTTPCheck            `json:"http_checks,omitempty" toml:"http_checks,omitempty"`
	Processes    []string                       `json:"processes,omitempty" toml:"processes,omitempty"`
}

type ServiceTCPCheck struct {
	Interval *api.Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout  *api.Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
}

type ServiceHTTPCheck struct {
	Interval          *api.Duration     `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout           *api.Duration     `json:"timeout,omitempty" toml:"timeout,omitempty"`
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

func (svc *Service) toMachineService() *api.MachineService {
	checks := make([]api.MachineCheck, 0, len(svc.TCPChecks)+len(svc.HTTPChecks))
	for _, tc := range svc.TCPChecks {
		checks = append(checks, *tc.toMachineCheck())
	}
	for _, hc := range svc.HTTPChecks {
		checks = append(checks, *hc.toMachineCheck())
	}
	return &api.MachineService{
		Protocol:     svc.Protocol,
		InternalPort: svc.InternalPort,
		Ports:        svc.Ports,
		Concurrency:  svc.Concurrency,
		Checks:       checks,
	}
}

func (chk *ServiceHTTPCheck) toMachineCheck() *api.MachineCheck {
	return &api.MachineCheck{
		Type:              api.Pointer("http"),
		Interval:          chk.Interval,
		Timeout:           chk.Timeout,
		HTTPMethod:        chk.HTTPMethod,
		HTTPPath:          chk.HTTPPath,
		HTTPProtocol:      chk.HTTPProtocol,
		HTTPSkipTLSVerify: chk.HTTPTLSSkipVerify,
		HTTPHeaders: lo.MapToSlice(
			chk.HTTPHeaders, func(k string, v string) api.MachineHTTPHeader {
				return api.MachineHTTPHeader{Name: k, Values: []string{v}}
			}),
	}
}

func (chk *ServiceHTTPCheck) String(port int) string {
	return fmt.Sprintf("http-%d-%v", port, chk.HTTPMethod)
}

func (chk *ServiceTCPCheck) toMachineCheck() *api.MachineCheck {
	return &api.MachineCheck{
		Type:     api.Pointer("tcp"),
		Interval: chk.Interval,
		Timeout:  chk.Timeout,
	}
}

func (chk *ServiceTCPCheck) String(port int) string {
	return fmt.Sprintf("tcp-%d", port)
}
