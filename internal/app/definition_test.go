package app

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/api"
)

var GetConfigJSON = []byte(`
{
  "env": {},
  "experimental": {
    "auto_rollback": true
  },
  "kill_signal": "SIGINT",
  "kill_timeout": 5,
  "processes": [],
  "services": [
    {
      "concurrency": {
        "hard_limit": 25,
        "soft_limit": 20,
        "type": "connections"
      },
      "http_checks": [],
      "internal_port": 8080,
      "ports": [
        {
          "force_https": true,
          "handlers": [
            "http"
          ],
          "port": 80
        },
        {
          "handlers": [
            "tls",
            "http"
          ],
          "port": 443
        }
      ],
      "processes": [
        "app"
      ],
      "protocol": "tcp",
      "script_checks": [],
      "tcp_checks": [
        {
          "grace_period": "1s",
          "interval": "15s",
          "restart_limit": 0,
          "timeout": "2s"
        }
      ]
    }
  ]
}
`)

func TestFromDefinition(t *testing.T) {
	definition := &api.Definition{}
	err := json.Unmarshal(GetConfigJSON, definition)
	assert.NoError(t, err)

	cfg, err := FromDefinition(definition)
	assert.NoError(t, err)

	assert.Equal(t, &Config{
		KillSignal:  "SIGINT",
		KillTimeout: 5,
		Experimental: &Experimental{
			AutoRollback: true,
		},
		Env: map[string]string{},
		Services: []Service{
			{
				InternalPort: 8080,
				Protocol:     "tcp",
				Concurrency: &api.MachineServiceConcurrency{
					Type:      "connections",
					HardLimit: 25,
					SoftLimit: 20,
				},
				Ports: []api.MachinePort{
					{
						Port:       api.Pointer(80),
						Handlers:   []string{"http"},
						ForceHttps: true,
					},
					{
						Port:     api.Pointer(443),
						Handlers: []string{"tls", "http"},
					},
				},
				Processes: []string{"app"},
				TCPChecks: []*ServiceTCPCheck{
					{
						Timeout:      api.MustParseDuration("2s"),
						RestartLimit: 0,
						Interval:     api.MustParseDuration("15s"),
						GracePeriod:  api.MustParseDuration("1s"),
					},
				},
			},
		},
	}, cfg)
}
