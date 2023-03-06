package appv2

import (
	"context"
	"io"

	"github.com/BurntSushi/toml"
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

// Use this type to unmarshal fly.toml with the goal of retreiving the app name only
type slimConfig struct {
	AppName string `toml:"app,omitempty"`
}

func (c *Config) DeterminePlatform(ctx context.Context, r io.ReadSeeker) (err error) {
	client := client.FromContext(ctx)
	cfg := &slimConfig{}
	_, err = toml.NewDecoder(r).Decode(&cfg)
	if err != nil {
		return err
	}

	basicApp, err := client.API().GetAppBasic(ctx, cfg.AppName)
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
