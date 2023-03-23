package scanner

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createDockerfile(t *testing.T, dockerfile string) func() {
	path := "/tmp/Dockerfile"

	assert.NoError(t, os.WriteFile(path, []byte(dockerfile), 0644))
	return func() {
		os.Remove(path)
	}
}

func TestDockerfileScanner(t *testing.T) {
	type testcase struct {
		name         string
		config       ScannerConfig
		expectedPort int
		dockerfile   string
	}

	testcases := []testcase{
		{
			name:         "fly.toml has port set, dockerfile has no port",
			dockerfile:   "FROM wordpress:latest",
			expectedPort: 5432,
			config: ScannerConfig{
				ExistingPort: 5432,
			},
		},
		{
			name:         "fly.toml no port set, dockerfile has no port",
			dockerfile:   "FROM wordpress:latest",
			expectedPort: 8080,
		},
		{
			name:         "fly.toml no port set, dockerfile has a port",
			expectedPort: 80,
			dockerfile:   "FROM wordpress:latest\nEXPOSE 80",
		},
		{
			name:         "fly.toml has a port set, dockerfile has a port",
			dockerfile:   "FROM wordpress:latest\nEXPOSE 80",
			expectedPort: 80,
			config: ScannerConfig{
				ExistingPort: 5432,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cleanup := createDockerfile(t, tc.dockerfile)
			defer cleanup()

			si, err := configureDockerfile("/tmp", &tc.config)
			assert.NoError(t, err)

			assert.Equal(t, tc.expectedPort, si.Port)
		})
	}
}
