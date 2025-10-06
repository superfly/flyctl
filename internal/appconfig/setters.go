package appconfig

// ** IMPORTANT **
// This methods are mainly called by `fly launch` with information provided by scanners

import (
	"time"

	fly "github.com/superfly/fly-go"
)

func (c *Config) SetInternalPort(port int) {
	switch {
	case c.HTTPService != nil:
		c.HTTPService.InternalPort = port
	case len(c.Services) > 0:
		c.Services[0].InternalPort = port
	}
}

func (c *Config) SetHttpCheck(path string, headers map[string]string) {
	check := &ServiceHTTPCheck{
		HTTPMethod:        fly.StringPointer("GET"),
		HTTPPath:          fly.StringPointer(path),
		HTTPProtocol:      fly.StringPointer("http"),
		HTTPTLSSkipVerify: fly.BoolPointer(false),
		Interval:          &fly.Duration{Duration: 10 * time.Second},
		Timeout:           &fly.Duration{Duration: 2 * time.Second},
		GracePeriod:       &fly.Duration{Duration: 5 * time.Second},
		HTTPHeaders:       headers,
	}

	switch {
	case c.HTTPService != nil:
		service := c.HTTPService
		service.HTTPChecks = append(service.HTTPChecks, check)
	case len(c.Services) > 0:
		service := &c.Services[0]
		service.HTTPChecks = append(service.HTTPChecks, check)
	}
}

func (c *Config) SetConcurrency(soft int, hard int) {
	concurrency := &fly.MachineServiceConcurrency{
		Type:      "connections",
		HardLimit: hard,
		SoftLimit: soft,
	}
	switch {
	case c.HTTPService != nil:
		c.HTTPService.Concurrency = concurrency
	case len(c.Services) > 0:
		service := &c.Services[0]
		service.Concurrency = concurrency
	}
}

func (c *Config) SetReleaseCommand(cmd string) {
	if c.Deploy == nil {
		c.Deploy = &Deploy{}
	}
	c.Deploy.ReleaseCommand = cmd
}

func (c *Config) SetDockerCommand(cmd string) {
	if c.Experimental == nil {
		c.Experimental = &Experimental{}
	}
	c.Experimental.Cmd = []string{cmd}
}

func (c *Config) SetKillSignal(signal string) {
	if signal != "" {
		c.KillSignal = &signal
	}
}

func (c *Config) SetDockerEntrypoint(entrypoint string) {
	if c.Experimental == nil {
		c.Experimental = &Experimental{}
	}
	c.Experimental.Entrypoint = []string{entrypoint}
}

func (c *Config) SetEnvVariable(name, value string) {
	if c.Env == nil {
		c.Env = make(map[string]string)
	}
	c.Env[name] = value
}

func (c *Config) SetEnvVariables(vals map[string]string) {
	for k, v := range vals {
		c.SetEnvVariable(k, v)
	}
}

func (c *Config) SetProcess(name, value string) {
	if c.Processes == nil {
		c.Processes = make(map[string]string)
	}
	c.Processes[name] = value
}

func (c *Config) SetStatics(statics []Static) {
	c.Statics = make([]Static, 0, len(statics))
	for _, static := range statics {
		c.Statics = append(c.Statics, Static{
			GuestPath:     static.GuestPath,
			UrlPrefix:     static.UrlPrefix,
			TigrisBucket:  static.TigrisBucket,
			IndexDocument: static.IndexDocument,
		})
	}
}

func (c *Config) SetMounts(volumes []Mount) {
	c.Mounts = volumes
}
