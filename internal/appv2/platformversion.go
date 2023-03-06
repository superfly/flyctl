package appv2

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/client"
)

const (
	// Config is versioned, initially, to separate nomad from machine apps without having to consult
	// the API
	AppsV1Platform   = "nomad"
	AppsV2Platform   = "machines"
	MachinesPlatform = AppsV2Platform
	NomadPlatform    = AppsV1Platform
)

// SetMachinesPlatform informs the TOML marshaller that this config is for the machines platform
func (c *Config) SetMachinesPlatform() {
	c.platformVersion = MachinesPlatform
}

// SetNomadPlatform informs the TOML marshaller that this config is for the nomad platform
func (c *Config) SetNomadPlatform() {
	c.platformVersion = NomadPlatform
}

func (c *Config) SetPlatformVersion(platform string) {
	c.platformVersion = platform
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
