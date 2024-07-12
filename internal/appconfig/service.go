package appconfig

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/sentry"
)

type Service struct {
	Protocol     string `json:"protocol,omitempty" toml:"protocol"`
	InternalPort int    `json:"internal_port,omitempty" toml:"internal_port"`
	// AutoStopMachines and AutoStartMachines should not have omitempty for TOML. The encoder
	// already omits nil since it can't be represented, and omitempty makes it omit false as well.
	AutoStopMachines   *fly.MachineAutostop           `json:"auto_stop_machines,omitempty" toml:"auto_stop_machines"`
	AutoStartMachines  *bool                          `json:"auto_start_machines,omitempty" toml:"auto_start_machines"`
	MinMachinesRunning *int                           `json:"min_machines_running,omitempty" toml:"min_machines_running,omitempty"`
	Ports              []fly.MachinePort              `json:"ports,omitempty" toml:"ports"`
	Concurrency        *fly.MachineServiceConcurrency `json:"concurrency,omitempty" toml:"concurrency"`
	TCPChecks          []*ServiceTCPCheck             `json:"tcp_checks,omitempty" toml:"tcp_checks,omitempty"`
	HTTPChecks         []*ServiceHTTPCheck            `json:"http_checks,omitempty" toml:"http_checks,omitempty"`
	MachineChecks      []*ServiceMachineCheck         `json:"machine_checks,omitempty" toml:"machine_checks,omitempty"`
	Processes          []string                       `json:"processes,omitempty" toml:"processes,omitempty"`
}

type ServiceTCPCheck struct {
	Interval    *fly.Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout     *fly.Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod *fly.Duration `toml:"grace_period,omitempty" json:"grace_period,omitempty"`
}

type ServiceHTTPCheck struct {
	Interval    *fly.Duration `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout     *fly.Duration `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod *fly.Duration `toml:"grace_period,omitempty" json:"grace_period,omitempty"`

	// HTTP Specifics
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPTLSServerName *string           `json:"tls_server_name,omitempty" toml:"tls_server_name,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

type ServiceMachineCheck struct {
	Command     []string      `json:"command,omitempty" toml:"command,omitempty"`
	Image       string        `json:"image,omitempty" toml:"image,omitempty"`
	Entrypoint  []string      `json:"entrypoint,omitempty" toml:"entrypoint,omitempty"`
	KillSignal  *string       `json:"kill_signal,omitempty" toml:"kill_signal,omitempty"`
	KillTimeout *fly.Duration `json:"kill_timeout,omitempty" toml:"kill_timeout,omitempty"`
}

type HTTPService struct {
	InternalPort int  `json:"internal_port,omitempty" toml:"internal_port,omitempty" validate:"required,numeric"`
	ForceHTTPS   bool `toml:"force_https,omitempty" json:"force_https,omitempty"`
	// AutoStopMachines and AutoStartMachines should not have omitempty for TOML; see the note in Service.
	AutoStopMachines   *fly.MachineAutostop           `json:"auto_stop_machines,omitempty" toml:"auto_stop_machines"`
	AutoStartMachines  *bool                          `json:"auto_start_machines,omitempty" toml:"auto_start_machines"`
	MinMachinesRunning *int                           `json:"min_machines_running,omitempty" toml:"min_machines_running,omitempty"`
	Processes          []string                       `json:"processes,omitempty" toml:"processes,omitempty"`
	Concurrency        *fly.MachineServiceConcurrency `toml:"concurrency,omitempty" json:"concurrency,omitempty"`
	TLSOptions         *fly.TLSOptions                `json:"tls_options,omitempty" toml:"tls_options,omitempty"`
	HTTPOptions        *fly.HTTPOptions               `json:"http_options,omitempty" toml:"http_options,omitempty"`
	HTTPChecks         []*ServiceHTTPCheck            `json:"checks,omitempty" toml:"checks,omitempty"`
	MachineChecks      []*ServiceMachineCheck         `json:"machine_checks,omitempty" toml:"machine_checks,omitempty"`
}

func (s *HTTPService) ToService() *Service {
	return &Service{
		Protocol:      "tcp",
		InternalPort:  s.InternalPort,
		Concurrency:   s.Concurrency,
		Processes:     s.Processes,
		HTTPChecks:    s.HTTPChecks,
		MachineChecks: s.MachineChecks,
		Ports: []fly.MachinePort{{
			Port:        fly.IntPointer(80),
			Handlers:    []string{"http"},
			ForceHTTPS:  s.ForceHTTPS,
			HTTPOptions: s.HTTPOptions,
		}, {
			Port:        fly.IntPointer(443),
			Handlers:    []string{"http", "tls"},
			HTTPOptions: s.HTTPOptions,
			TLSOptions:  s.TLSOptions,
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

func (svc *Service) toMachineService() *fly.MachineService {
	s := &fly.MachineService{
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

func (chk *ServiceHTTPCheck) toMachineCheck() *fly.MachineCheck {
	return &fly.MachineCheck{
		Type:              fly.Pointer("http"),
		Interval:          chk.Interval,
		Timeout:           chk.Timeout,
		GracePeriod:       chk.GracePeriod,
		HTTPMethod:        chk.HTTPMethod,
		HTTPPath:          chk.HTTPPath,
		HTTPProtocol:      chk.HTTPProtocol,
		HTTPSkipTLSVerify: chk.HTTPTLSSkipVerify,
		HTTPTLSServerName: chk.HTTPTLSServerName,
		HTTPHeaders: lo.MapToSlice(
			chk.HTTPHeaders, func(k string, v string) fly.MachineHTTPHeader {
				return fly.MachineHTTPHeader{Name: k, Values: []string{v}}
			}),
	}
}

func (chk *ServiceHTTPCheck) String(port int) string {
	return fmt.Sprintf("http-%d-%v", port, chk.HTTPMethod)
}

func (chk *ServiceTCPCheck) toMachineCheck() *fly.MachineCheck {
	return &fly.MachineCheck{
		Type:        fly.Pointer("tcp"),
		Interval:    chk.Interval,
		Timeout:     chk.Timeout,
		GracePeriod: chk.GracePeriod,
	}
}

func (chk *ServiceTCPCheck) String(port int) string {
	return fmt.Sprintf("tcp-%d", port)
}

func serviceFromMachineService(ctx context.Context, ms fly.MachineService, processes []string) *Service {
	var (
		tcpChecks  []*ServiceTCPCheck
		httpChecks []*ServiceHTTPCheck
	)
	for _, check := range ms.Checks {
		switch *check.Type {
		case "tcp":
			tcpChecks = append(tcpChecks, tcpCheckFromMachineCheck(check))
		case "http":
			httpChecks = append(httpChecks, httpCheckFromMachineCheck(ctx, check))
		default:
			sentry.CaptureException(fmt.Errorf("unknown check type '%s' when converting from machine service", *check.Type), sentry.WithTraceID(ctx))
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

func tcpCheckFromMachineCheck(mc fly.MachineCheck) *ServiceTCPCheck {
	return &ServiceTCPCheck{
		Interval:    mc.Interval,
		Timeout:     mc.Timeout,
		GracePeriod: nil,
	}
}

func httpCheckFromMachineCheck(ctx context.Context, mc fly.MachineCheck) *ServiceHTTPCheck {
	headers := make(map[string]string)
	for _, h := range mc.HTTPHeaders {
		if len(h.Values) > 0 {
			headers[h.Name] = h.Values[0]
		}
		if len(h.Values) > 1 {
			sentry.CaptureException(fmt.Errorf("bug: more than one header value provided by MachineCheck, but can only support one value for fly.toml"), sentry.WithTraceID(ctx))
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
