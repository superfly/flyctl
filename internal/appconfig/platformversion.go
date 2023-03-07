package appconfig

import "fmt"

func (c *Config) EnsureV2Config() error {
	return c.v2UnmarshalError
}

// SetMachinesPlatform informs the TOML marshaller that this config is for the machines platform
func (c *Config) SetMachinesPlatform() error {
	if c.v2UnmarshalError != nil {
		return c.v2UnmarshalError
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
	case NomadPlatform:
		return c.SetNomadPlatform()
	case "":
		return fmt.Errorf("Empty value as platform version")
	default:
		return fmt.Errorf("Unknown platform version: '%s'", platform)
	}
}

// ForMachines is true when the config is intended for the machines platform
func (c *Config) ForMachines() bool {
	return c.platformVersion == MachinesPlatform
}
