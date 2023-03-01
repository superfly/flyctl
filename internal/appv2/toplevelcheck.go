package appv2

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/sentry"
	"golang.org/x/exp/slices"
)

type ToplevelCheck struct {
	Port              *int              `json:"port,omitempty" toml:"port,omitempty"`
	Type              *string           `json:"type,omitempty" toml:"type,omitempty"`
	Interval          *api.Duration     `json:"interval,omitempty" toml:"interval,omitempty"`
	Timeout           *api.Duration     `json:"timeout,omitempty" toml:"timeout,omitempty"`
	GracePeriod       *api.Duration     `json:"grace_period,omitempty" toml:"grace_period,omitempty"`
	HTTPMethod        *string           `json:"method,omitempty" toml:"method,omitempty"`
	HTTPPath          *string           `json:"path,omitempty" toml:"path,omitempty"`
	HTTPProtocol      *string           `json:"protocol,omitempty" toml:"protocol,omitempty"`
	HTTPTLSSkipVerify *bool             `json:"tls_skip_verify,omitempty" toml:"tls_skip_verify,omitempty"`
	HTTPHeaders       map[string]string `json:"headers,omitempty" toml:"headers,omitempty"`
}

func topLevelCheckFromMachineCheck(mc api.MachineCheck) *ToplevelCheck {
	headers := make(map[string]string)
	for _, h := range mc.HTTPHeaders {
		if len(h.Values) > 0 {
			headers[h.Name] = h.Values[0]
		}
		if len(h.Values) > 1 {
			sentry.CaptureException(fmt.Errorf("bug: more than one header value provided by MachineCheck, but can only support one value for fly.toml"))
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
		HTTPHeaders:       headers,
	}
}

func (chk *ToplevelCheck) toMachineCheck() (*api.MachineCheck, error) {
	if chk.Type == nil || !slices.Contains([]string{"http", "tcp"}, *chk.Type) {
		return nil, fmt.Errorf("Missing or invalid check type, must be 'http' or 'tcp'")
	}

	res := &api.MachineCheck{
		Type:              chk.Type,
		Port:              chk.Port,
		Interval:          chk.Interval,
		Timeout:           chk.Timeout,
		GracePeriod:       chk.GracePeriod,
		HTTPPath:          chk.HTTPPath,
		HTTPProtocol:      chk.HTTPProtocol,
		HTTPSkipTLSVerify: chk.HTTPTLSSkipVerify,
		HTTPHeaders: lo.MapToSlice(
			chk.HTTPHeaders, func(k string, v string) api.MachineHTTPHeader {
				return api.MachineHTTPHeader{Name: k, Values: []string{v}}
			}),
	}
	if chk.HTTPMethod != nil {
		res.HTTPMethod = api.Pointer(strings.ToUpper(*chk.HTTPMethod))
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
