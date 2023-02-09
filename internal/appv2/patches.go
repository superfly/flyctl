package appv2

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type patchFuncType func(map[string]any) (map[string]any, error)

var configPatches = []patchFuncType{
	patchEnv,
	patchServices,
	patchProcesses,
	patchExperimental,
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
	if raw, ok := cfg["env"]; ok {
		env := map[string]string{}

		switch cast := raw.(type) {
		case map[string]string:
			env = cast
		case map[string]any:
			for k, v := range cast {
				if stringVal, ok := v.(string); ok {
					env[k] = stringVal
				} else {
					env[k] = fmt.Sprintf("%v", v)
				}
			}
		default:
			return nil, fmt.Errorf("Do not know how to process 'env' section of type: %T", cast)
		}

		cfg["env"] = env
	}
	return cfg, nil
}

func patchProcesses(cfg map[string]any) (map[string]any, error) {
	if raw, ok := cfg["processes"]; ok {
		switch cast := raw.(type) {
		case []any, []map[string]any:
			// GQL GetConfig returns an empty array when there are not processes
			delete(cfg, "processes")
		case map[string]any:
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
				// Remove experiemntal section if empty
				delete(cfg, "experimental")
			}
		default:
			return nil, fmt.Errorf("Unknown type: %T", cast)
		}
	}
	return cfg, nil
}

func patchServices(cfg map[string]any) (map[string]any, error) {
	if raw, ok := cfg["services"]; ok {
		services, err := ensureArrayOfMap(raw)
		if err != nil {
			return nil, fmt.Errorf("Error processing services: %w", err)
		}

		for idx, service := range services {
			service, err := _patchService(service)
			if err != nil {
				return nil, err
			}
			services[idx] = service
		}
		cfg["services"] = services
	}
	return cfg, nil
}

func _patchService(service map[string]any) (map[string]any, error) {
	if concurrency, ok := service["concurrency"]; ok {
		switch cast := concurrency.(type) {
		case string:
			// parse old "{soft},{hard}" strings
			left, right, ok := strings.Cut(cast, ",")
			if !ok {
				return nil, fmt.Errorf("Unknown value '%s' for concurrency limits", cast)
			}

			softLimit, err := strconv.Atoi(left)
			if err != nil {
				return nil, fmt.Errorf("Can not convert '%s': %w", cast, err)
			}

			hardLimit, err := strconv.Atoi(right)
			if err != nil {
				return nil, fmt.Errorf("Can not convert '%s': %w", cast, err)
			}

			service["concurrency"] = map[string]any{
				"type":       "requests",
				"hard_limit": hardLimit,
				"soft_limit": softLimit,
			}
		case map[string]any:
			// Nothing to do here
		default:
			return nil, fmt.Errorf("Unknown type for service concurrency: %T", cast)
		}
	}

	if rawPorts, ok := service["ports"]; ok {
		ports, err := ensureArrayOfMap(rawPorts)
		if err != nil {
			return nil, fmt.Errorf("Error processing ports: %T", rawPorts)
		}

		for idx, port := range ports {
			if portN, ok := port["port"]; ok {
				switch cast := portN.(type) {
				case string:
					n, err := strconv.Atoi(cast)
					if err != nil {
						return nil, fmt.Errorf("Can not convert port '%s' to integer: %w", cast, err)
					}
					port["port"] = n
				case float64:
					port["port"] = int(cast)
				case int64:
					port["port"] = int(cast)
				default:
					return nil, fmt.Errorf("Unknown type for port number: %T", cast)
				}
			}
			ports[idx] = port
		}
		service["ports"] = ports
	}

	for _, checkType := range []string{"tcp_checks", "http_checks"} {
		if rawChecks, ok := service[checkType]; ok {
			checks, err := _patchChecks(rawChecks)
			if err != nil {
				return nil, fmt.Errorf("Error processing tcp_checks: %T", rawChecks)
			}
			if len(checks) > 0 {
				service[checkType] = checks
			} else {
				delete(service, checkType)
			}
		}
	}

	return service, nil
}

func _patchChecks(rawChecks any) ([]map[string]any, error) {
	checks, err := ensureArrayOfMap(rawChecks)
	if err != nil {
		return nil, err
	}

	for idx, check := range checks {
		for _, attr := range []string{"interval", "timeout"} {
			if v, ok := check[attr]; ok {
				switch cast := v.(type) {
				case string:
					// Nothing to do here
				case int64:
					// Convert milliseconds to microseconds as expected by api.ParseDuration
					check[attr] = time.Duration(cast) * time.Millisecond
				}
			}
		}
		checks[idx] = check
	}
	return checks, nil
}

func ensureArrayOfMap(raw any) ([]map[string]any, error) {
	out := []map[string]any{}
	switch cast := raw.(type) {
	case []any:
		for _, rawItem := range cast {
			item, ok := rawItem.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Can not cast '%s' of type '%t' as map[string]any", rawItem, rawItem)
			}
			out = append(out, item)
		}
	case []map[string]any:
		out = cast
	default:
		return nil, fmt.Errorf("Unknown type '%T'", cast)
	}
	return out, nil
}
