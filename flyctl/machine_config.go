package flyctl

type MachineConfig struct {
	AppName string
	Config  map[string]interface{}
}

func NewMachineConfig() *MachineConfig {
	return &MachineConfig{
		Config: map[string]interface{}{},
	}
}

func (mc *MachineConfig) SetEnvVariables(vals map[string]string) {
	var env map[string]string

	if rawEnv, ok := mc.Config["env"]; ok {
		if castEnv, ok := rawEnv.(map[string]string); ok {
			env = castEnv
		}
	}
	if env == nil {
		env = map[string]string{}
	}

	for k, v := range vals {
		env[k] = v
	}

	mc.Config["env"] = env
}
