package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

type patchFuncType func(map[string]any) (map[string]any, error)

var configPatches = []patchFuncType{
	patchEnv,
	patchConcurrency,
	patchProcesses,
	patchExperimental,
}

// LoadConfig loads the app config at the given path.
func LoadConfig(ctx context.Context, path string) (cfg *Config, err error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg, err = unmarshalTOML(buf)
	if err != nil {
		return nil, err
	}

	cfg.FlyTomlPath = path
	return cfg, nil
}

func unmarshalTOML(buf []byte) (*Config, error) {
	cfgMap := map[string]any{}
	if err := toml.Unmarshal(buf, &cfgMap); err != nil {
		return nil, err
	}

	if err := toml.Unmarshal(buf, &Config{}); err != nil {
		return nil, fmt.Errorf("Can not decode into app.Config: %w", err)
	}
	return applyPatches(cfgMap)
}

func applyPatches(cfgMap map[string]any) (*Config, error) {
	// Migrate whatever we found in old fly.toml files to newish format
	for _, patchFunc := range configPatches {
		var err error
		cfgMap, err = patchFunc(cfgMap)
		if err != nil {
			return nil, err
		}
	}

	newbuf, err := json.Marshal(cfgMap)
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	return cfg, json.Unmarshal(newbuf, cfg)
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

func patchProcesses(cfg map[string]any) (map[string]any, error) {
	if raw, ok := cfg["processes"]; ok {
		switch cast := raw.(type) {
		case []any:
			delete(cfg, "processes")
		case map[string]string:
			// Nothing to do here
		default:
			return nil, fmt.Errorf("Unknown processes type: %T", cast)
		}
	}
	return cfg, nil
}

func patchExperimental(cfg map[string]any) (map[string]any, error) {
	if raw, ok := cfg["experimental"]; ok {
		switch cast := raw.(type) {
		case map[string]any:
			if len(cast) == 0 {
				delete(cfg, "experimental")
			}
		default:
			return nil, fmt.Errorf("Unknown type: %T", cast)
		}
	}
	return cfg, nil
}
