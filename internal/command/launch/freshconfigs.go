package launch

import (
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
)

var (
	v2CheckTimeout  = api.MustParseDuration("2s")
	v2CheckInterval = api.MustParseDuration("15s")
	v2GracePeriod   = api.MustParseDuration("5s")
)

func freshV2Config(appName string, srcCfg *appconfig.Config) (*appconfig.Config, error) {
	newCfg := appconfig.NewConfig()
	newCfg.AppName = appName
	newCfg.Build = srcCfg.Build
	newCfg.PrimaryRegion = srcCfg.PrimaryRegion
	newCfg.HTTPService = &appconfig.HTTPService{
		InternalPort:      8080,
		ForceHTTPS:        true,
		AutoStartMachines: api.Pointer(true),
		AutoStopMachines:  api.Pointer(true),
	}
	newCfg.Checks = map[string]*appconfig.ToplevelCheck{
		"alive": {
			Type:        api.Pointer("tcp"),
			Timeout:     v2CheckTimeout,
			Interval:    v2CheckInterval,
			GracePeriod: v2GracePeriod,
		},
	}

	if err := newCfg.SetMachinesPlatform(); err != nil {
		return nil, err
	}
	return newCfg, nil
}

func freshV1Config(appName string, srcCfg *appconfig.Config, definition *api.Definition) (*appconfig.Config, error) {
	newCfg, err := appconfig.FromDefinition(definition)
	if err != nil {
		return nil, err
	}
	newCfg.AppName = appName
	newCfg.Build = srcCfg.Build
	newCfg.PrimaryRegion = srcCfg.PrimaryRegion

	if err := newCfg.SetNomadPlatform(); err != nil {
		return nil, err
	}
	return newCfg, nil
}
