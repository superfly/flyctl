package appconfig

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStrictValidate(t *testing.T) {
	tests := []struct {
		name                     string
		config                   string
		wantUnrecognizedSections []string
		wantUnrecognizedKeys     map[string][]string
	}{
		{
			name: "valid config",
			config: `
				app = "test-app"
				primary_region = "iad"

				[build]
					builder = "dockerfile"

				[env]
					NODE_ENV = "production"
			`,
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     nil,
		},
		{
			name: "unrecognized top-level section",
			config: `
				app = "test-app"

				[unknown_section]
					key = "value"
			`,
			wantUnrecognizedSections: []string{"unknown_section"},
			wantUnrecognizedKeys:     nil,
		},
		{
			name: "unrecognized key in build section",
			config: `
				app = "test-app"

				[build]
					builder = "dockerfile"
					unknown_key = "value"
			`,
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     map[string][]string{"build": {"unknown_key"}},
		},
		{
			name: "unrecognized key in checks value",
			config: `
				app = "test-app"

				[checks.health_check]
					type = "http"
					invalid_key = "value"
			`,
			wantUnrecognizedSections: nil,
			wantUnrecognizedKeys:     map[string][]string{"checks.health_check": {"invalid_key"}},
		},
		{
			name: "real-world example",
			config: `
				app = "bla"
				primary_region = "mia"
				console_command = "bin/rails console"

				[build]
				dockerfile = "Dockerfile.web"
				build-target = "deploy"

				[build.args]
				APP_URL = "https://staging.floridacims.org"
				RAILS_ENV = "staging"
				RACK_ENV = "staging"
				APPUID = "1000"
				APPGID = "1000"

				[deploy]
				processes = ["app"]
				release_command = "./bin/rails db:prepare"
				strategy = "bluegreen"

				[env]
				RAILS_MAX_THREADS = 5

				[http_service]
				processes = ["app"]
				internal_port = 3000
				auto_stop_machines = "suspend"
				auto_start_machines = true
				min_machines_running = 1

				[[http_service.checks]]
				processes = ['app']
				grace_period = "10s"
				interval = "30s"
				protocol = "http"
				method = "GET"
				timeout = "5s"
				path = "/up"

				[[http_machine.checks]]
				processes = ['app']
				grace_period = "30s"
				image = "curlimages/curl"
				entrypoint = ["/bin/sh", "-c"]
				command = ["curl http://[$FLY_TEST_MACHINE_IP]/up | grep 'background-color: green'"]
				kill_signal = "SIGKILL"
				kill_timeout = "5s"

				[[http_service.machine_checks]]
				processes = ['app']
				grace_period = "30s"
				image = "curlimages/curl"
				entrypoint = ["/bin/sh", "-c"]
				command = ["curl http://[$FLY_TEST_MACHINE_IP]/up | grep 'background-color: green'"]
				kill_signal = "SIGKILL"
				kill_timeout = "5s"

				[http_service.concurrency]
				processes = ['app']
				type = "requests"
				soft_limit = 50
				hard_limit = 70

				[http_service.http_options]
				h2_backend = true
				xyz = "123"

				[[vm]]
				processes = ["app"]
				size = "shared-cpu-2x"
				memory = '2gb'

				[[vm]]
				processes = ["worker"]
				size = "shared-cpu-2x"
				memory = '2gb'

				[[statics]]
				guest_path = "/rails/public"
				url_prefix = "/"

				[processes]
				app = "bundle exec rails s -b 0.0.0.0 -p 3000"
				worker = "bundle exec sidekiq"

				[checks.my_check_bla]
				type = "http"
				grace_period = "30s"
				invalid_key = 123
			`,
			wantUnrecognizedSections: []string{"http_machine"},
			wantUnrecognizedKeys: map[string][]string{
				"http_service.checks[0]":         {"processes"},
				"checks.my_check_bla":            {"invalid_key"},
				"deploy":                         {"processes"},
				"http_service.machine_checks[0]": {"grace_period", "processes"},
				"http_service.concurrency":       {"processes"},
				"http_service.http_options":      {"xyz"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "fly-*.toml")
			assert.NoError(t, err)
			defer os.Remove(f.Name())

			_, err = f.WriteString(tt.config)
			assert.NoError(t, err)

			rawConfig, err := LoadConfigAsMap(f.Name())
			assert.NoError(t, err)

			result := StrictValidate(rawConfig)

			assert.ElementsMatch(t, result.UnrecognizedSections, tt.wantUnrecognizedSections)

			assert.Len(t, result.UnrecognizedKeys, len(tt.wantUnrecognizedKeys))
			for section, keys := range tt.wantUnrecognizedKeys {
				gotKeys, ok := result.UnrecognizedKeys[section]
				assert.True(t, ok)

				assert.ElementsMatch(t, gotKeys, keys)
			}
		})
	}
}
