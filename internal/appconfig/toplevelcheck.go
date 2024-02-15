package appconfig

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/sentry"
)

type ToplevelCheck struct {
	Port              *int              `json:"port,omitempty" toml:"port,omitempty"`
	Type              *string           `json:"type,omitempty" toml:"type,omitempty"`
	Interval          *fly.Duration     `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout           *fly.Duration     `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod       *fly.Duration     `json:"grace_period,omitempty" toml:"grace_period,omitempty"`
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPTLSServerName *string           `json:"tls_server_name,omitempty" toml:"tls_server_name,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
	Processes         []string          `json:"processes,omitempty" toml:"processes,omitempty"`
}

func topLevelCheckFromMachineCheck(ctx context.Context, mc fly.MachineCheck) *ToplevelCheck {
	headers := make(map[string]string)
	for _, h := range mc.HTTPHeaders {
		if len(h.Values) > 0 {
			headers[h.Name] = h.Values[0]
		}
		if len(h.Values) > 1 {
			sentry.CaptureException(fmt.Errorf("bug: more than one header value provided by MachineCheck, but can only support one value for fly.toml"), sentry.WithTraceID(ctx))
		}
	}
	return &ToplevelCheck{
		Port:              mc.Port,
		Type:              mc.Type,
		Interval:          mc.Interval,
		Timeout:           mc.Timeout,
		GracePeriod:       mc.GracePeriod,
		HTTPMethod:        mc.HTTPMethod,
		HTTPPath:          mc.HTTPPath,
		HTTPProtocol:      mc.HTTPProtocol,
		HTTPTLSSkipVerify: mc.HTTPSkipTLSVerify,
		HTTPTLSServerName: mc.HTTPTLSServerName,
		HTTPHeaders:       headers,
	}
}

func (chk *ToplevelCheck) toMachineCheck() (*fly.MachineCheck, error) {
	if chk.Type == nil || !slices.Contains([]string{"http", "tcp"}, *chk.Type) {
		return nil, fmt.Errorf("Missing or invalid check type, must be 'http' or 'tcp'")
	}

	res := &fly.MachineCheck{
		Type:              chk.Type,
		Port:              chk.Port,
		Interval:          chk.Interval,
		Timeout:           chk.Timeout,
		GracePeriod:       chk.GracePeriod,
		HTTPPath:          chk.HTTPPath,
		HTTPProtocol:      chk.HTTPProtocol,
		HTTPSkipTLSVerify: chk.HTTPTLSSkipVerify,
		HTTPTLSServerName: chk.HTTPTLSServerName,
	}
	if chk.HTTPMethod != nil {
		res.HTTPMethod = fly.Pointer(strings.ToUpper(*chk.HTTPMethod))
	}
	if len(chk.HTTPHeaders) > 0 {
		res.HTTPHeaders = lo.MapToSlice(
			chk.HTTPHeaders, func(k string, v string) fly.MachineHTTPHeader {
				return fly.MachineHTTPHeader{Name: k, Values: []string{v}}
			})
	}
	return res, nil
}

func (chk *ToplevelCheck) String() string {
	chkType := "none"
	if chk.Type != nil {
		chkType = *chk.Type
	}
	switch chkType {
	case "tcp":
		return fmt.Sprintf("tcp-%d", chk.Port)
	case "http":
		return fmt.Sprintf("http-%d-%v", chk.Port, chk.HTTPMethod)
	default:
		return fmt.Sprintf("%s-%d", chkType, chk.Port)
	}
}
