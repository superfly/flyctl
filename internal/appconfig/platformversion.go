package appconfig

// SetMachinesPlatform informs the TOML marshaller that this config is for the machines platform
func (c *Config) SetMachinesPlatform() error {
	if c.v2UnmarshalError != nil {
		return c.v2UnmarshalError
	}
	return nil
}
