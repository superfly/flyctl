package legacy

import (
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/appconfig"
)

func freshV2Config(appName string, srcCfg *appconfig.Config) (*appconfig.Config, error) {
	newCfg := appconfig.NewConfig()
	newCfg.AppName = appName
	newCfg.Build = srcCfg.Build
	newCfg.PrimaryRegion = srcCfg.PrimaryRegion
	newCfg.Restart = &appconfig.Restart{
		Policy: appconfig.RestartPolicyAlways,
	}
	newCfg.HTTPService = &appconfig.HTTPService{
		InternalPort:       8080,
		ForceHTTPS:         true,
		AutoStartMachines:  api.Pointer(true),
		AutoStopMachines:   api.Pointer(true),
		MinMachinesRunning: api.Pointer(0),
		Processes:          []string{"app"},
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
