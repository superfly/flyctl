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
	patchExperimental,
	patchTopLevelChecks,
	patchCompute,
	patchMounts,
	patchMetrics,
	patchTopFields,
	patchBuild,
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

func patchTopFields(cfg map[string]any) (map[string]any, error) {
	if raw, ok := cfg["kill_timeout"]; ok {
		cfg["kill_timeout"] = _castDuration(raw, time.Second)
	}
	return cfg, nil
}

func patchEnv(cfg map[string]any) (map[string]any, error) {
	raw, ok := cfg["env"]
	if !ok {
		return cfg, nil
	}
	env, err := _patchEnv(raw)
	if err != nil {
		return nil, err
	}
	cfg["env"] = env
	return cfg, nil
}

func _patchEnv(raw any) (map[string]string, error) {
	env := map[string]string{}

	switch cast := raw.(type) {
	case []map[string]any:
		for _, raw2 := range cast {
			env2, err := _patchEnv(raw2)
			if err != nil {
				return nil, err
			}
			for k, v := range env2 {
				env[k] = v
			}
		}
	case []any:
		for _, raw2 := range cast {
			env2, err := _patchEnv(raw2)
			if err != nil {
				return nil, err
			}
			for k, v := range env2 {
				env[k] = v
			}
		}
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

	return env, nil
}

func patchProcesses(cfg map[string]any) (map[string]any, error) {
	if raw, ok := cfg["processes"]; ok {
		switch cast := raw.(type) {
		case []any:
			processes := []map[string]any{}
			for _, item := range cast {
				if v, ok := item.(map[string]any); ok {
					processes = append(processes, v)
				}
			}
			cfg["processes"] = processes
			return patchProcesses(cfg)

		case []map[string]any:
			processes := make(map[string]string)
			for _, item := range cast {
				if rawk, kok := item["name"]; kok {
					k := castToString(rawk)
					if v, vok := item["command"]; vok {
						processes[k] = castToString(v)
					} else {
						processes[k] = ""
					}
				}
			}
			if len(processes) > 0 {
				cfg["processes"] = processes
			} else {
				delete(cfg, "processes")
			}
		case map[string]any:
			// Nothing to do here
		default:
			return nil, fmt.Errorf("Unknown processes type: %T", cast)
		}
	}
	return cfg, nil
}

func patchBuild(cfg map[string]any) (map[string]any, error) {
	raw, ok := cfg["build"]
	if !ok {
		return cfg, nil
	}

	cast, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Build section of unknown type: %T", raw)
	}

	for k, v := range cast {
		switch k {
		case "build_target":
			cast["build-target"] = v
		}
	}

	if len(cast) == 0 {
		delete(cfg, "build")
	} else {
		cfg["build"] = cast
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
		return nil, fmt.Errorf("Experimental section of unknown type: %T", raw)
	}

	metrics := map[string]any{}

	for k, v := range cast {
		switch k {
		case "cmd", "entrypoint", "exec":
			if n, err := stringOrSliceToSlice(v, k); err != nil {
				return nil, err
			} else {
				cast[k] = n
			}
		case "kill_timeout":
			if _, ok := cfg["kill_timeout"]; !ok {
				cfg["kill_timeout"] = _castDuration(v, time.Second)
			}
		case "metrics_port", "metrics_path":
			metrics[strings.TrimPrefix(k, "metrics_")] = v
		}
	}

	if len(cast) == 0 {
		delete(cfg, "experimental")
	} else {
		cfg["experimental"] = cast
	}

	if _, ok := cfg["metrics"]; !ok && len(metrics) > 0 {
		cfg["metrics"] = metrics
	}

	return cfg, nil
}

func patchCompute(cfg map[string]any) (map[string]any, error) {
	var compute []map[string]any
	for _, k := range []string{"compute", "computes", "vm"} {
		if raw, ok := cfg[k]; ok {
			cast, err := ensureArrayOfMap(raw)
			if err != nil {
				return nil, fmt.Errorf("Error processing compute: %w", err)
			}
			delete(cfg, k)
			compute = append(compute, cast...)
		}
	}
	for idx, c := range compute {
		if v, ok := c["memory"]; ok {
			compute[idx]["memory"] = castToString(v)
		}
	}
	cfg["vm"] = compute
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
			delete(cfg, k)
			mounts = append(mounts, cast...)
		}
	}
	for idx, x := range mounts {
		if v, ok := x["initial_size"]; ok {
			mounts[idx]["initial_size"] = castToString(v)
		}
	}
	cfg["mounts"] = mounts
	return cfg, nil
}

func patchMetrics(cfg map[string]any) (map[string]any, error) {
	var metrics []map[string]any
	for _, k := range []string{"metric", "metrics"} {
		if raw, ok := cfg[k]; ok {
			cast, err := ensureArrayOfMap(raw)
			if err != nil {
				return nil, fmt.Errorf("Error processing mounts: %w", err)
			}
			metrics = append(metrics, cast...)
		}
	}
	cfg["metrics"] = metrics
	return cfg, nil
}

func patchTopLevelChecks(cfg map[string]any) (map[string]any, error) {
	raw, ok := cfg["checks"]
	if !ok {
		return cfg, nil
	}

	checks := map[string]any{}

	switch cast := raw.(type) {
	case map[string]any:
		var err error
		checks, err = _patchTopLevelChecks(cast)
		if err != nil {
			return nil, err
		}
	case []any:
		for _, raw2 := range cast {
			cast2, ok := raw2.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("check item of unknown type: %T", raw2)
			}

			name, ok := cast2["name"].(string)
			if !ok {
				return nil, fmt.Errorf("check item name not a string")
			}

			subChecks, err := _patchTopLevelChecks(map[string]any{name: raw2})
			if err != nil {
				return nil, err
			}
			for k, v := range subChecks {
				checks[k] = v
			}
		}
	default:
		return nil, fmt.Errorf("'checks' section of unknown type: %T", raw)
	}

	if len(checks) == 0 {
		delete(cfg, "checks")
		return cfg, nil
	}

	cfg["checks"] = checks
	return cfg, nil
}

func _patchTopLevelChecks(raw any) (map[string]any, error) {
	cast, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unknown check type: %T", raw)
	}

	for k, rawCheck := range cast {
		castCheck, ok := rawCheck.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("'checks' section of unknown type: %T", rawCheck)
		}

		check, err := _patchCheck(castCheck)
		if err != nil {
			return nil, err
		}
		cast[k] = check
	}
	return cast, nil
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
				return nil, fmt.Errorf("Error processing %s: %w", checkType, err)
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
	for _, attr := range []string{"interval", "timeout", "grace_period"} {
		if v, ok := check[attr]; ok {
			check[attr] = _castDuration(v, time.Millisecond)
		}
	}

	if v, ok := check["headers"]; ok {
		headers, err := _patchCheckHeaders(v)
		if err != nil {
			return nil, err
		}

		if len(headers) > 0 {
			check["headers"] = headers
		} else {
			delete(check, "headers")
		}
	}
	return check, nil
}

func _patchCheckHeaders(rawHeaders any) (map[string]string, error) {
	headers := make(map[string]string)
	switch cast := rawHeaders.(type) {
	case []any:
		acc := []map[string]any{}
		for _, item := range cast {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Can't cast %#v into map[string]any", item)
			}
			acc = append(acc, m)
		}
		return _patchCheckHeaders(acc)

	case []map[string]any:
		for _, m := range cast {
			if k, ok := m["name"]; ok {
				if v, ok := m["value"]; ok {
					headers[castToString(k)] = castToString(v)
				} else {
					headers[castToString(k)] = ""
				}
			} else {
				return nil, fmt.Errorf("Unsupported headers format %#v", m)
			}
		}

	case map[string]any:
		for name, rawValue := range cast {
			headers[name] = castToString(rawValue)
		}

	default:
		return nil, fmt.Errorf("Unsupported headers format: %#v", rawHeaders)
	}

	return headers, nil
}

func _castDuration(v any, shift time.Duration) (ret *string) {
	switch cast := v.(type) {
	case *string:
		return cast
	case string:
		if cast == "" {
			return nil
		}
		return &cast
	case float64:
		d := time.Duration(cast)
		if shift > 0 {
			d = d * shift
		}
		str := d.String()
		return &str
	case int64:
		d := time.Duration(cast)
		if shift > 0 {
			d = d * shift
		}
		str := d.String()
		return &str
	}
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

func castToString(rawValue any) string {
	if v, ok := rawValue.(string); ok {
		return v
	}
	return fmt.Sprintf("%v", rawValue)
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
