package appconfig

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
	patchHTTPService,
	patchExperimental,
	patchTopLevelChecks,
	patchMounts,
}

func applyPatches(cfgMap map[string]any) (*Config, error) {
	cfgMap, err := patchRoot(cfgMap)
	if err != nil {
		return nil, err
	}
	return mapToConfig(cfgMap)
}

func mapToConfig(cfgMap map[string]any) (*Config, error) {
	cfg := NewConfig()
	newbuf, err := json.Marshal(cfgMap)
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(newbuf, cfg)
}

// Migrate whatever we found in old fly.toml files to newish format
func patchRoot(cfgMap map[string]any) (map[string]any, error) {
	var err error
	for _, patchFunc := range configPatches {
		cfgMap, err = patchFunc(cfgMap)
		if err != nil {
			return cfgMap, err
		}
	}
	return cfgMap, nil
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
	raw, ok := cfg["experimental"]
	if !ok {
		return cfg, nil
	}

	cast, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Experimental section of unknown type: %T", cast)
	}

	for k, v := range cast {
		switch k {
		case "cmd", "entrypoint", "exec":
			if n, err := stringOrSliceToSlice(v, k); err != nil {
				return nil, err
			} else {
				cast[k] = n
			}
		}
	}

	if len(cast) == 0 {
		delete(cfg, "experimental")
	} else {
		cfg["experimental"] = cast
	}

	return cfg, nil
}

func patchMounts(cfg map[string]any) (map[string]any, error) {
	var mounts []map[string]any
	for _, k := range []string{"mount", "mounts"} {
		if raw, ok := cfg[k]; ok {
			cast, err := ensureArrayOfMap(raw)
			if err != nil {
				return nil, fmt.Errorf("Error processing mounts: %w", err)
			}
			mounts = append(mounts, cast...)
		}
	}
	cfg["mounts"] = mounts
	return cfg, nil
}

func patchHTTPService(cfg map[string]any) (map[string]any, error) {
	raw, ok := cfg["http_service"]
	if !ok {
		return cfg, nil
	}

	cast, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("http_service section of unknown type: %T", raw)
	}

	if err := _patchProcessesField(cast); err != nil {
		return nil, err
	}
	cfg["http_service"] = cast
	return cfg, nil
}

func patchTopLevelChecks(cfg map[string]any) (map[string]any, error) {
	raw, ok := cfg["checks"]
	if !ok {
		return cfg, nil
	}

	cast, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'checks' section of unknown type: %T", cast)
	}

	for k, rawCheck := range cast {
		castCheck, ok := rawCheck.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("'checks' section of unknown type: %T", castCheck)
		}

		check, err := _patchCheck(castCheck)
		if err != nil {
			return nil, err
		}
		cast[k] = check
	}
	return cfg, nil
}

func patchServices(cfg map[string]any) (map[string]any, error) {
	if raw, ok := cfg["services"]; ok {
		services, err := ensureArrayOfMap(raw)
		if err != nil {
			return nil, fmt.Errorf("Error processing services: %w", err)
		}

		var newServices []map[string]any
		for _, service := range services {
			service, err := _patchService(service)
			if err != nil {
				return nil, err
			}
			if len(service) != 0 {
				newServices = append(newServices, service)
			}
		}
		cfg["services"] = newServices
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
				casted_port, err := castToInt(portN)
				if err != nil {
					return nil, err
				}

				port["port"] = casted_port
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

	if rawInternalPort, ok := service["internal_port"]; ok {
		internal_port, err := castToInt(rawInternalPort)
		if err != nil {
			return nil, err
		}

		service["internal_port"] = internal_port
	}

	if err := _patchProcessesField(service); err != nil {
		return nil, err
	}

	return service, nil
}

func _patchChecks(rawChecks any) ([]map[string]any, error) {
	checks, err := ensureArrayOfMap(rawChecks)
	if err != nil {
		return nil, err
	}

	for idx, rawCheck := range checks {
		check, err := _patchCheck(rawCheck)
		if err != nil {
			return nil, err
		}
		checks[idx] = check
	}
	return checks, nil
}

func _patchCheck(check map[string]any) (map[string]any, error) {
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
	if v, ok := check["headers"]; ok {
		switch cast := v.(type) {
		case []any:
			if len(cast) == 0 {
				delete(check, "headers")
			}
		}
	}
	return check, nil
}

func _patchProcessesField(in map[string]any) error {
	rawProcesses, ok := in["processes"]
	if !ok {
		return nil
	}

	n, err := stringOrSliceToSlice(rawProcesses, "processes")
	if err != nil {
		return err
	}

	in["processes"] = n
	return nil
}

func castToInt(num any) (int, error) {
	switch cast := num.(type) {
	case string:
		n, err := strconv.Atoi(cast)
		if err != nil {
			return 0, fmt.Errorf("Can not convert '%s' to integer: %w", cast, err)
		}
		return n, nil

	case float32:
		return int(cast), nil
	case float64:
		return int(cast), nil
	case int:
		return cast, nil
	case int32:
		return int(cast), nil
	case int64:
		return int(cast), nil
	case uint:
		return int(cast), nil
	case uint32:
		return int(cast), nil
	case uint64:
		return int(cast), nil
	default:
		return 0, fmt.Errorf("Unknown type for cast: %T", cast)

	}
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
	case map[string]any:
		out = append(out, cast)
	default:
		return nil, fmt.Errorf("Unknown type '%T'", cast)
	}
	return out, nil
}

func stringOrSliceToSlice(input any, fieldName string) ([]string, error) {
	if input == nil {
		return nil, nil
	}

	if c, ok := input.([]string); ok {
		return c, nil
	} else if c, ok := input.(string); ok {
		return []string{c}, nil
	} else if c, ok := input.([]any); ok {
		ret := []string{}
		for _, v := range c {
			if cv, ok := v.(string); ok {
				ret = append(ret, cv)
			} else {
				return nil, fmt.Errorf("could not cast %v of type %T to string on %s", v, v, fieldName)
			}
		}
		return ret, nil
	} else {
		return nil, fmt.Errorf("could not cast %v of type %T to []string on %s", input, input, fieldName)
	}
}
