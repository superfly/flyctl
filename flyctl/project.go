package flyctl

import (
	"bufio"
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

type Project struct {
	ProjectDir       string
	cfg              *viper.Viper
	configFileLoaded bool
}

func NewProject(configFile string) *Project {
	v := viper.New()
	v.SetConfigFile(configFile)

	return &Project{
		cfg:        v,
		ProjectDir: path.Dir(configFile),
	}
}

func LoadProject(configFile string) (*Project, error) {
	configFile, err := ResolveConfigFileFromPath(configFile)
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.SetConfigType("toml")

	p := &Project{
		cfg:        v,
		ProjectDir: path.Dir(configFile),
	}

	err = v.ReadInConfig()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		p.configFileLoaded = true
	}

	return p, nil
}

func (p *Project) WriteConfig() error {
	if err := os.MkdirAll(path.Dir(p.cfg.ConfigFileUsed()), 0777); err != nil {
		return err
	}

	return p.cfg.WriteConfig()
}

func (p *Project) SafeWriteConfig() error {
	err := p.cfg.SafeWriteConfig()
	if os.IsNotExist(err) {
		return p.WriteConfig()
	}
	return err
}

func (p *Project) WriteConfigToPath(filename string) error {
	p.cfg.SetConfigFile(filename)
	return p.cfg.WriteConfigAs(filename)
}

func (p *Project) WriteConfigAsString() string {
	var buf bytes.Buffer
	toml.NewEncoder(bufio.NewWriter(&buf)).Encode(p.cfg.AllSettings())
	return buf.String()
}

func (p *Project) ConfigFileLoaded() bool {
	return p.configFileLoaded
}

func (p *Project) ConfigFilePath() string {
	return p.cfg.ConfigFileUsed()
}

func (p *Project) AppName() string {
	return p.cfg.GetString("app")
}

func (p *Project) SetAppName(name string) {
	p.cfg.Set("app", name)
}

func (p *Project) HasBuildConfig() bool {
	return p.cfg.IsSet("build")
}

func (p *Project) Builder() string {
	return p.cfg.GetString("build.builder")
}

func (p *Project) SetBuilder(name string) {
	p.cfg.Set("build.builder", name)
}

func (p *Project) BuildArgs() map[string]string {
	args := p.cfg.GetStringMapString("build")
	delete(args, "builder")
	return args
}

func (p *Project) Services() []api.Service {
	services := []api.Service{}
	cfgServices, ok := p.cfg.Get("services").([]interface{})
	if ok {
		for _, in := range cfgServices {
			inSvc := in.(map[string]interface{})
			svc := api.Service{}
			if val, ok := inSvc["protocol"]; ok {
				svc.Protocol = strings.ToUpper(cast.ToString(val))
			}
			if val, ok := inSvc["internal_port"]; ok {
				svc.InternalPort = cast.ToInt(val)
			}
			if val, ok := inSvc["port"]; ok {
				for rawPort, val := range cast.ToStringMap(val) {
					portN, err := strconv.Atoi(rawPort)
					if err != nil {
						terminal.Warnf("Error parsing port number '%s': %s", rawPort, err)
						continue
					}
					port := api.PortHandler{
						Port: portN,
					}

					portHandler := cast.ToStringMap(val)

					handlers := []string{}
					if val, ok := portHandler["handlers"]; ok {
						for _, handler := range cast.ToStringSlice(val) {
							handlers = append(handlers, strings.ToUpper(handler))
						}
					}
					port.Handlers = handlers
					svc.Ports = append(svc.Ports, port)
				}
			}

			if val, ok := inSvc["tcp_check"]; ok {
				for _, val := range cast.ToSlice(val) {
					checkIn := cast.ToStringMap(val)
					check := api.Check{
						Type: "TCP",
					}
					if val, ok := checkIn["name"]; ok {
						check.Name = new(string)
						*check.Name = cast.ToString(val)
					}
					if val, ok := checkIn["interval"]; ok {
						duration, err := time.ParseDuration(cast.ToString(val))
						if err != nil {
							terminal.Warnf("Error parsing check interval '%s': %s", val, err)
							continue
						}
						check.Interval = new(uint64)
						*check.Interval = uint64(duration.Milliseconds())
					}
					if val, ok := checkIn["timeout"]; ok {
						duration, err := time.ParseDuration(cast.ToString(val))
						if err != nil {
							terminal.Warnf("Error parsing check timeout '%s': %s", val, err)
							continue
						}
						check.Timeout = new(uint64)
						*check.Timeout = uint64(duration.Milliseconds())
					}
					svc.Checks = append(svc.Checks, check)
				}
			}

			if val, ok := inSvc["http_check"]; ok {
				for _, val := range cast.ToSlice(val) {
					checkIn := cast.ToStringMap(val)
					check := api.Check{
						Type: "HTTP",
					}
					if val, ok := checkIn["name"]; ok {
						check.Name = new(string)
						*check.Name = cast.ToString(val)
					}
					if val, ok := checkIn["interval"]; ok {
						duration, err := time.ParseDuration(cast.ToString(val))
						if err != nil {
							terminal.Warnf("Error parsing check interval '%s': %s", val, err)
							continue
						}
						check.Interval = new(uint64)
						*check.Interval = uint64(duration.Milliseconds())
					}
					if val, ok := checkIn["timeout"]; ok {
						duration, err := time.ParseDuration(cast.ToString(val))
						if err != nil {
							terminal.Warnf("Error parsing check timeout '%s': %s", val, err)
							continue
						}
						check.Timeout = new(uint64)
						*check.Timeout = uint64(duration.Milliseconds())
					}

					if val, ok := checkIn["method"]; ok {
						check.HTTPMethod = new(string)
						*check.HTTPMethod = strings.ToUpper(cast.ToString(val))
					}
					if val, ok := checkIn["path"]; ok {
						check.HTTPPath = new(string)
						*check.HTTPPath = cast.ToString(val)
					}
					if val, ok := checkIn["protocol"]; ok {
						check.HTTPProtocol = new(string)
						*check.HTTPProtocol = strings.ToUpper(cast.ToString(val))
					}
					if val, ok := checkIn["skip_verify_tls"]; ok {
						check.HTTPSkipTLSVerify = new(bool)
						*check.HTTPSkipTLSVerify = cast.ToBool(val)
					}
					if val, ok := checkIn["headers"]; ok {
						for n, v := range cast.ToStringMapString(val) {
							check.HTTPHeaders = append(check.HTTPHeaders, api.HTTPHeader{Name: n, Value: v})
						}
					}
					svc.Checks = append(svc.Checks, check)
				}
			}

			services = append(services, svc)
		}

	}

	return services
}

func (p *Project) SetServices(services []api.Service) {
	s := []map[string]interface{}{}

	for _, x := range services {
		svc := map[string]interface{}{
			"protocol":      strings.ToLower(x.Protocol),
			"internal_port": x.InternalPort,
		}

		ports := []interface{}{}
		for _, port := range x.Ports {
			x := map[string]interface{}{}
			x["port"] = port.Port
			handlers := []string{}
			for _, handler := range port.Handlers {
				handlers = append(handlers, strings.ToLower(handler))
			}
			x["handlers"] = handlers
			ports = append(ports, port)
		}
		svc["ports"] = ports

		tcpChecks := []interface{}{}
		httpChecks := []interface{}{}

		for _, check := range x.Checks {
			x := map[string]interface{}{}

			if check.Name != nil {
				x["name"] = *check.Name
			}

			if check.Interval != nil {
				x["interval"] = time.Duration(*check.Interval * uint64(time.Millisecond.Nanoseconds())).String()
			}

			if check.Timeout != nil {
				x["timeout"] = time.Duration(*check.Timeout * uint64(time.Millisecond.Nanoseconds())).String()
			}

			if check.Type == "TCP" {
				tcpChecks = append(tcpChecks, x)
			} else if check.Type == "HTTP" {
				if check.HTTPMethod != nil {
					x["method"] = strings.ToLower(*check.HTTPMethod)
				}
				if check.HTTPPath != nil {
					x["path"] = *check.HTTPPath
				}
				if check.HTTPProtocol != nil {
					x["protocol"] = strings.ToLower(*check.HTTPProtocol)
				}
				if check.HTTPSkipTLSVerify != nil {
					x["skip_verify_tls"] = *check.HTTPSkipTLSVerify
				}
				if len(check.HTTPHeaders) > 0 {
					headers := map[string]string{}
					for _, header := range check.HTTPHeaders {
						headers[header.Name] = header.Value
					}
				}
				httpChecks = append(httpChecks, x)
			}
		}

		if len(tcpChecks) > 0 {
			svc["tcp_check"] = tcpChecks
		}
		if len(httpChecks) > 0 {
			svc["http_check"] = httpChecks
		}

		s = append(s, svc)
	}

	p.cfg.Set("services", s)
}

const defaultConfigFileName = "fly.toml"

func ResolveConfigFileFromPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	// path is a directory, append default config file name
	if filepath.Ext(p) == "" {
		p = path.Join(p, defaultConfigFileName)
	}

	return p, nil
}

func ConfigFileExistsAtPath(p string) (bool, error) {
	p, err := ResolveConfigFileFromPath(p)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	return !os.IsNotExist(err), nil
}
