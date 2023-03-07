package appv2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/client"
)

// SetMachinesPlatform informs the TOML marshaller that this config is for the machines platform
func (c *Config) SetMachinesPlatform() error {
	if c.parseError != nil {
		return c.parseError
	}
	c.platformVersion = MachinesPlatform
	return nil
}

// SetNomadPlatform informs the TOML marshaller that this config is for the nomad platform
func (c *Config) SetNomadPlatform() error {
	if len(c.RawDefinition) == 0 {
		return fmt.Errorf("Can't set platformVersion to Nomad on an empty RawDefinition")
	}
	c.platformVersion = NomadPlatform
	return nil
}

func (c *Config) SetPlatformVersion(platform string) error {
	switch platform {
	case MachinesPlatform:
		return c.SetMachinesPlatform()
	default:
		return c.SetNomadPlatform()
	}
}

// ForMachines is true when the config is intended for the machines platform
func (c *Config) ForMachines() bool {
	return c.platformVersion == MachinesPlatform
}

func (c *Config) DeterminePlatform(ctx context.Context) (err error) {
	client := client.FromContext(ctx)
	if c.AppName == "" {
		return fmt.Errorf("Can't determine platform without an application name")
	}

	basicApp, err := client.API().GetAppBasic(ctx, c.AppName)
	if err != nil {
		return err
	}

	if basicApp.PlatformVersion == MachinesPlatform {
		c.SetMachinesPlatform()
	} else {
		c.SetNomadPlatform()
	}

	return
}
