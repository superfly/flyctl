package machine

import (
	"encoding/json"

	"github.com/superfly/flyctl/api"
)

func CloneConfig(orig api.MachineConfig) (*api.MachineConfig, error) {
	config := &api.MachineConfig{}

	data, err := json.Marshal(orig)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, err
}
