package appconfig

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/sentry"
)

type Service struct {
	Protocol     string `json:"protocol,omitempty" toml:"protocol"`
	InternalPort int    `json:"internal_port,omitempty" toml:"internal_port"`
	// AutoStopMachines and AutoStartMachines should not have omitempty for TOML. The encoder
	// already omits nil since it can't be represented, and omitempty makes it omit false as well.
	AutoStopMachines   *bool                          `json:"auto_stop_machines,omitempty" toml:"auto_stop_machines"`
	AutoStartMachines  *bool                          `json:"auto_start_machines,omitempty" toml:"auto_start_machines"`
	MinMachinesRunning *int                           `json:"min_machines_running,omitempty" toml:"min_machines_running,omitempty"`
	Ports              []api.MachinePort              `json:"ports,omitempty" toml:"ports"`
	Concurrency        *api.MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
	TCPChecks          []*ServiceTCPCheck             `json:"tcp_checks,omitempty" toml:"tcp_checks,omitempty"`
	HTTPChecks         []*ServiceHTTPCheck            `json:"http_checks,omitempty" toml:"http_checks,omitempty"`
	Processes          []string                       `json:"processes,omitempty" toml:"processes,omitempty"`
}

type ServiceTCPCheck struct {
	Interval    *api.Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout     *api.Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod *api.Duration `toml:"grace_period,omitempty" json:"grace_period,omitempty"`
}

type ServiceHTTPCheck struct {
	Interval    *api.Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout     *api.Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod *api.Duration `toml:"grace_period,omitempty" json:"grace_period,omitempty"`

	// HTTP Specifics
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPTLSServerName *string           `json:"tls_server_name,omitempty" toml:"tls_server_name,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

type HTTPService struct {
	InternalPort int  `json:"internal_port,omitempty" toml:"internal_port,omitempty" validate:"required,numeric"`
	ForceHTTPS   bool `toml:"force_https,omitempty" json:"force_https,omitempty"`
	// AutoStopMachines and AutoStartMachines should not have omitempty for TOML; see the note in Service.
	AutoStopMachines   *bool                          `json:"auto_stop_machines,omitempty" toml:"auto_stop_machines"`
	AutoStartMachines  *bool                          `json:"auto_start_machines,omitempty" toml:"auto_start_machines"`
	MinMachinesRunning *int                           `json:"min_machines_running,omitempty" toml:"min_machines_running,omitempty"`
	Processes          []string                       `json:"processes,omitempty" toml:"processes,omitempty"`
	Concurrency        *api.MachineServiceConcurrency `toml:"concurrency,omitempty" json:"concurrency,omitempty"`
	TLSOptions         *api.TLSOptions                `json:"tls_options,omitempty" toml:"tls_options,omitempty"`
	HTTPOptions        *api.HTTPOptions               `json:"http_options,omitempty" toml:"http_options,omitempty"`
	ProxyProtoOptions  *api.ProxyProtoOptions         `json:"proxy_proto_options,omitempty" toml:"proxy_proto_options,omitempty"`
	HTTPChecks         []*ServiceHTTPCheck            `json:"checks,omitempty" toml:"checks,omitempty"`
}

func (s *HTTPService) ToService() *Service {
	return &Service{
		Protocol:     "tcp",
		InternalPort: s.InternalPort,
		Concurrency:  s.Concurrency,
		Processes:    s.Processes,
		HTTPChecks:   s.HTTPChecks,
		Ports: []api.MachinePort{{
			Port:              api.IntPointer(80),
			Handlers:          []string{"http"},
			ForceHTTPS:        s.ForceHTTPS,
			HTTPOptions:       s.HTTPOptions,
			ProxyProtoOptions: s.ProxyProtoOptions,
		}, {
			Port:              api.IntPointer(443),
			Handlers:          []string{"http", "tls"},
			HTTPOptions:       s.HTTPOptions,
			TLSOptions:        s.TLSOptions,
			ProxyProtoOptions: s.ProxyProtoOptions,
		}},
		AutoStopMachines:   s.AutoStopMachines,
		AutoStartMachines:  s.AutoStartMachines,
		MinMachinesRunning: s.MinMachinesRunning,
	}
}

func (c *Config) AllServices() (services []Service) {
	if c.HTTPService != nil {
		services = append(services, *c.HTTPService.ToService())
	}
	services = append(services, c.Services...)
	return services
}

func (svc *Service) toMachineService() *api.MachineService {
	s := &api.MachineService{
		Protocol:           svc.Protocol,
		InternalPort:       svc.InternalPort,
		Ports:              svc.Ports,
		Concurrency:        svc.Concurrency,
		Autostop:           svc.AutoStopMachines,
		Autostart:          svc.AutoStartMachines,
		MinMachinesRunning: svc.MinMachinesRunning,
	}

	for _, tc := range svc.TCPChecks {
		s.Checks = append(s.Checks, *tc.toMachineCheck())
	}
	for _, hc := range svc.HTTPChecks {
		s.Checks = append(s.Checks, *hc.toMachineCheck())
	}
	return s
}

func (chk *ServiceHTTPCheck) toMachineCheck() *api.MachineCheck {
	return &api.MachineCheck{
		Type:              api.Pointer("http"),
		Interval:          chk.Interval,
		Timeout:           chk.Timeout,
		GracePeriod:       chk.GracePeriod,
		HTTPMethod:        chk.HTTPMethod,
		HTTPPath:          chk.HTTPPath,
		HTTPProtocol:      chk.HTTPProtocol,
		HTTPSkipTLSVerify: chk.HTTPTLSSkipVerify,
		HTTPTLSServerName: chk.HTTPTLSServerName,
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
		Type:        api.Pointer("tcp"),
		Interval:    chk.Interval,
		Timeout:     chk.Timeout,
		GracePeriod: chk.GracePeriod,
	}
}

func (chk *ServiceTCPCheck) String(port int) string {
	return fmt.Sprintf("tcp-%d", port)
}

func serviceFromMachineService(ms api.MachineService, processes []string) *Service {
	var (
		tcpChecks  []*ServiceTCPCheck
		httpChecks []*ServiceHTTPCheck
	)
	for _, check := range ms.Checks {
		switch *check.Type {
		case "tcp":
			tcpChecks = append(tcpChecks, tcpCheckFromMachineCheck(check))
		case "http":
			httpChecks = append(httpChecks, httpCheckFromMachineCheck(check))
		default:
			sentry.CaptureException(fmt.Errorf("unknown check type '%s' when converting from machine service", *check.Type))
		}
	}
	return &Service{
		Protocol:           ms.Protocol,
		InternalPort:       ms.InternalPort,
		AutoStopMachines:   ms.Autostop,
		AutoStartMachines:  ms.Autostart,
		MinMachinesRunning: ms.MinMachinesRunning,
		Ports:              ms.Ports,
		Concurrency:        ms.Concurrency,
		TCPChecks:          tcpChecks,
		HTTPChecks:         httpChecks,
		Processes:          processes,
	}
}

func tcpCheckFromMachineCheck(mc api.MachineCheck) *ServiceTCPCheck {
	return &ServiceTCPCheck{
		Interval:    mc.Interval,
		Timeout:     mc.Timeout,
		GracePeriod: nil,
	}
}

func httpCheckFromMachineCheck(mc api.MachineCheck) *ServiceHTTPCheck {
	headers := make(map[string]string)
	for _, h := range mc.HTTPHeaders {
		if len(h.Values) > 0 {
			headers[h.Name] = h.Values[0]
		}
		if len(h.Values) > 1 {
			sentry.CaptureException(fmt.Errorf("bug: more than one header value provided by MachineCheck, but can only support one value for fly.toml"))
		}
	}
	return &ServiceHTTPCheck{
		Interval:          mc.Interval,
		Timeout:           mc.Timeout,
		GracePeriod:       nil,
		HTTPMethod:        mc.HTTPMethod,
		HTTPPath:          mc.HTTPPath,
		HTTPProtocol:      mc.HTTPProtocol,
		HTTPTLSSkipVerify: mc.HTTPSkipTLSVerify,
		HTTPTLSServerName: mc.HTTPTLSServerName,
		HTTPHeaders:       headers,
	}
}
