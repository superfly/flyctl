package appconfig

// ** IMPORTANT **
// The main purpose of the functions in this file is to serve as a compatibility layer between
// V1 and V2 applications, considering Nomad apps (V1) uses RawDefinition for its configuration
// While V2 uses the fields in Config struct
//
// This methods are mainly called by `fly launch` with information provided by scanners

import (
	"fmt"
	"time"

	"github.com/superfly/flyctl/api"
)

func (c *Config) SetInternalPort(port int) {
	c.v1SetInternalPort(port)
	if len(c.Services) > 0 {
		c.Services[0].InternalPort = port
	}
}

func (c *Config) v1SetInternalPort(port int) {
	if raw, ok := c.RawDefinition["services"]; ok {
		services, _ := ensureArrayOfMap(raw)
		if len(services) > 0 {
			services[0]["internal_port"] = port
		}
	}
}

func (c *Config) SetHttpCheck(path string) {
	c.v1SetHttpCheck(path)
	if len(c.Services) > 0 {
		service := &c.Services[0]
		service.HTTPChecks = append(service.HTTPChecks, &ServiceHTTPCheck{
			HTTPMethod:        api.StringPointer("GET"),
			HTTPPath:          api.StringPointer(path),
			HTTPProtocol:      api.StringPointer("http"),
			HTTPTLSSkipVerify: api.BoolPointer(false),
			Interval:          &api.Duration{Duration: 10 * time.Second},
			Timeout:           &api.Duration{Duration: 2 * time.Second},
			GracePeriod:       &api.Duration{Duration: 5 * time.Second},
			RestartLimit:      0,
		})
	}
}

func (c *Config) v1SetHttpCheck(path string) {
	if raw, ok := c.RawDefinition["services"]; ok {
		services, _ := ensureArrayOfMap(raw)
		if len(services) > 0 {
			services[0]["http_checks"] = []map[string]interface{}{{
				"interval":        10000,
				"grace_period":    "5s",
				"method":          "get",
				"path":            path,
				"protocol":        "http",
				"restart_limit":   0,
				"timeout":         2000,
				"tls_skip_verify": false,
			}}
		}
	}
}

func (c *Config) SetConcurrency(soft int, hard int) {
	c.v1SetConcurrency(soft, hard)
	if len(c.Services) > 0 {
		service := &c.Services[0]
		if service.Concurrency == nil {
			service.Concurrency = &api.MachineServiceConcurrency{}
		}
		service.Concurrency.Type = "connections"
		service.Concurrency.HardLimit = hard
		service.Concurrency.SoftLimit = soft
	}
}

func (c *Config) v1SetConcurrency(soft int, hard int) {
	if raw, ok := c.RawDefinition["services"]; ok {
		services, _ := ensureArrayOfMap(raw)
		if len(services) > 0 {
			services[0]["concurrency"] = map[string]any{
				"hard_limit": hard,
				"soft_limit": soft,
				"type":       "connections",
			}
		}
	}
}

func (c *Config) SetReleaseCommand(cmd string) {
	c.v1SetReleaseCommand(cmd)
	if c.Deploy == nil {
		c.Deploy = &Deploy{}
	}
	c.Deploy.ReleaseCommand = cmd
}

func (c *Config) v1SetReleaseCommand(cmd string) {
	if raw, ok := c.RawDefinition["deploy"]; ok {
		if cast, ok := raw.(map[string]string); ok {
			cast["release_command"] = cmd
		} else if cast, ok := raw.(map[string]any); ok {
			cast["release_command"] = cmd
		}
	} else {
		c.RawDefinition["deploy"] = map[string]string{"release_command": cmd}
	}
}

func (c *Config) SetDockerCommand(cmd string) {
	c.v1SetDockerCommand(cmd)
	if c.Experimental == nil {
		c.Experimental = &Experimental{}
	}
	c.Experimental.Cmd = []string{cmd}
}

func (c *Config) v1SetDockerCommand(cmd string) {
	if raw, ok := c.RawDefinition["experimental"]; ok {
		if cast, ok := raw.(map[string]string); ok {
			cast["cmd"] = cmd
		} else if cast, ok := raw.(map[string]any); ok {
			cast["cmd"] = cmd
		}
	} else {
		c.RawDefinition["experimental"] = map[string]string{"cmd": cmd}
	}
}

func (c *Config) SetKillSignal(signal string) {
	c.RawDefinition["kill_signal"] = signal
	c.KillSignal = signal
}

func (c *Config) SetDockerEntrypoint(entrypoint string) {
	c.v1SetDockerEntrypoint(entrypoint)
	if c.Experimental == nil {
		c.Experimental = &Experimental{}
	}
	c.Experimental.Entrypoint = []string{entrypoint}
}

func (c *Config) v1SetDockerEntrypoint(entrypoint string) {
	if raw, ok := c.RawDefinition["experimental"]; ok {
		if cast, ok := raw.(map[string]string); ok {
			cast["entrypoint"] = entrypoint
		} else if cast, ok := raw.(map[string]any); ok {
			cast["entrypoint"] = entrypoint
		}
	} else {
		c.RawDefinition["experimental"] = map[string]string{"entrypoint": entrypoint}
	}
}

func (c *Config) SetEnvVariable(name, value string) {
	c.v1SetEnvVariable(name, value)
	if c.Env == nil {
		c.Env = make(map[string]string)
	}
	c.Env[name] = value
}

func (c *Config) SetEnvVariables(vals map[string]string) {
	c.v1SetEnvVariables(vals)
	for k, v := range vals {
		c.SetEnvVariable(k, v)
	}
}

func (c *Config) v1SetEnvVariable(name, value string) {
	c.v1SetEnvVariables(map[string]string{name: value})
}

func (c *Config) v1SetEnvVariables(vals map[string]string) {
	env := c.v1GetEnvVariables()
	for k, v := range vals {
		env[k] = v
	}
	c.RawDefinition["env"] = env
}

func (c *Config) v1GetEnvVariables() map[string]string {
	env := map[string]string{}

	if rawEnv, ok := c.RawDefinition["env"]; ok {
		// we get map[string]interface{} when unmarshaling toml, and map[string]string from SetEnvVariables.
		// Support them both :vomit:
		switch castEnv := rawEnv.(type) {
		case map[string]string:
			env = castEnv
		case map[string]interface{}:
			for k, v := range castEnv {
				if stringVal, ok := v.(string); ok {
					env[k] = stringVal
				} else {
					env[k] = fmt.Sprintf("%v", v)
				}
			}
		}
	}

	return env
}

func (c *Config) SetProcess(name, value string) {
	c.v1SetProcess(name, value)
	if c.Processes == nil {
		c.Processes = make(map[string]string)
	}
	c.Processes[name] = value
}

func (c *Config) v1SetProcess(name, value string) {
	if raw, ok := c.RawDefinition["processes"]; ok {
		if cast, ok := raw.(map[string]string); ok {
			cast[name] = value
		} else if cast, ok := raw.(map[string]any); ok {
			cast[name] = value
		}
	} else {
		c.RawDefinition["processes"] = map[string]string{name: value}
	}
}

func (c *Config) SetStatics(statics []Static) {
	c.RawDefinition["statics"] = statics
	c.Statics = make([]Static, 0, len(statics))
	for _, static := range statics {
		c.Statics = append(c.Statics, Static{
			GuestPath: static.GuestPath,
			UrlPrefix: static.UrlPrefix,
		})
	}
}

func (c *Config) SetVolumes(volumes []Volume) {
	c.RawDefinition["mounts"] = volumes
	// FIXME: "mounts" section is confusing, it is plural but only allows one mount
	if len(volumes) > 0 {
		c.Mounts = &Volume{
			Source:      volumes[0].Source,
			Destination: volumes[0].Destination,
		}
	}
}
