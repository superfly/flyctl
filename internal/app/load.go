package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/BurntSushi/toml"
)

type patchFuncType func(map[string]any) (map[string]any, error)

var configPatches = []patchFuncType{
	patchEnv,
	patchConcurrency,
}

func unmarshalTOML(r io.ReadSeeker) (*Config, error) {
	cfgMap := map[string]any{}
	_, err := toml.NewDecoder(r).Decode(&cfgMap)
	if err != nil {
		return nil, err
	}

	// Warn of TOML parsing errors
	_, err = r.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}
	if _, err = toml.NewDecoder(r).Decode(&Config{}); err != nil {
		return nil, err
	}

	for _, patchFunc := range configPatches {
		var err error
		cfgMap, err = patchFunc(cfgMap)
		if err != nil {
			return nil, err
		}
	}

	buf, err := json.Marshal(cfgMap)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	err = json.Unmarshal(buf, cfg)
	return cfg, err
}

func patchEnv(cfg map[string]any) (map[string]any, error) {
	if rawEnv, ok := cfg["env"]; ok {
		env := map[string]string{}

		switch castEnv := rawEnv.(type) {
		case map[string]string:
			env = castEnv
		case map[string]any:
			for k, v := range castEnv {
				if stringVal, ok := v.(string); ok {
					env[k] = stringVal
				} else {
					env[k] = fmt.Sprintf("%v", v)
				}
			}
		default:
			return nil, fmt.Errorf("Do not know how to process 'env' section of type: %T", castEnv)
		}

		cfg["env"] = env
	}
	return cfg, nil
}

func patchConcurrency(cfg map[string]any) (map[string]any, error) {
	return cfg, nil
}
