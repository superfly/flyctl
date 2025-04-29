package deploy

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/inmem"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/iostreams"
)

//go:embed testdata
var testdata embed.FS

// Test that `printMachineConfig` correctly processes machine config templates
func TestPrintMachineConfig(t *testing.T) {
	testCases := []struct {
		name              string
		configTemplate    string
		expectedContainer string
	}{
		{
			name: "Template with single container",
			configTemplate: `{
				"containers": {
					"app": {
						"image": "will-be-overridden",
						"env": {
							"TEST_VAR": "test_value"
						}
					}
				}
			}`,
			expectedContainer: "app",
		},
		{
			name: "Template with multiple containers",
			configTemplate: `{
				"containers": {
					"app": {
						"image": "will-be-overridden",
						"env": {
							"TEST_VAR": "test_value"
						}
					},
					"sidecar": {
						"image": "redis:latest",
						"env": {
							"REDIS_PORT": "6379"
						}
					}
				}
			}`,
			expectedContainer: "app",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test context
			var buf bytes.Buffer
			ios := &iostreams.IOStreams{Out: &buf, ErrOut: &buf}
			ctx := iostreams.NewContext(context.Background(), ios)

			// Create a temporary file for the config template
			tmpfile, err := os.CreateTemp("", "machine-config-*.json")
			require.NoError(t, err)
			defer os.Remove(tmpfile.Name())

			_, err = tmpfile.Write([]byte(tc.configTemplate))
			require.NoError(t, err)
			err = tmpfile.Close()
			require.NoError(t, err)

			// Setup the flags
			ctx = flag.NewContext(ctx, nil)
			ctx = flag.WithValue(ctx, "machine-config-template", tmpfile.Name())

			// Create app config and deployment image
			appConfig := &appconfig.Config{
				AppName: "test-app",
				Experimental: &appconfig.Experimental{
					MachineConfig: tmpfile.Name(),
				},
			}

			img := &imgsrc.DeploymentImage{
				Tag: "registry.fly.io/test-app:deployment-123456789",
			}

			// Call the function
			err = printMachineConfig(ctx, appConfig, img)
			require.NoError(t, err)

			// Check the output
			output := buf.String()
			assert.Contains(t, output, "Processing machine configuration template")
			assert.Contains(t, output, "Machine Config for Process Group: app")

			// Parse the JSON output to verify the container structure
			jsonStart := strings.Index(output, "{")
			jsonEnd := strings.LastIndex(output, "}") + 1
			jsonOutput := output[jsonStart:jsonEnd]

			var machineConfig fly.MachineConfig
			err = json.Unmarshal([]byte(jsonOutput), &machineConfig)
			require.NoError(t, err)

			// Verify that containers are present and properly configured
			assert.NotNil(t, machineConfig.Containers, "Containers should be present in the output")
			assert.Contains(t, machineConfig.Containers, tc.expectedContainer, "The 'app' container should be included")
			assert.Equal(t, img.Tag, machineConfig.Image, "The image should be preserved")
		})
	}
}

func TestCommand_Execute(t *testing.T) {
	makeTerminalLoggerQuiet(t)

	dir := t.TempDir()
	fsys, _ := fs.Sub(testdata, "testdata/basic")
	if err := copyFS(fsys, dir); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	var buf bytes.Buffer
	cmd := New()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--image", "test-registry.fly.io/my-image:deployment-00000000000000000000000000"})

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, &iostreams.IOStreams{Out: &buf, ErrOut: &buf})
	ctx = task.NewWithContext(ctx)
	ctx = logger.NewContext(ctx, logger.New(&buf, logger.Info, true))

	server := inmem.NewServer()
	server.CreateApp(&fly.App{
		Name:         "test-basic",
		Organization: fly.Organization{Slug: "my-org"},
	})
	if err := server.CreateImage(context.Background(), "test-basic", "test-registry.fly.io/my-image:deployment-00000000000000000000000000", &fly.Image{
		ID:             "IMAGE1",
		Ref:            "test-registry.fly.io/my-image:deployment-00000000000000000000000000",
		CompressedSize: "1000",
	}); err != nil {
		t.Fatal(err)
	}

	ctx = flyutil.NewContextWithClient(ctx, server.Client())
	ctx = flapsutil.NewContextWithClient(ctx, server.FlapsClient("test-basic"))

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
}

// copyFS writes the contents of a file system to a destination path on disk.
func copyFS(fsys fs.FS, dst string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		target := filepath.Join(dst, filepath.FromSlash(path))
		if err != nil {
			return err
		}

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o666)
	})
}

func chdir(tb testing.TB, dir string) {
	tb.Helper()

	prev, err := os.Getwd()
	if err != nil {
		tb.Fatalf("cannot read working directory: %s", err)
	}
	if err := os.Chdir(dir); err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		tb.Helper()
		if err := os.Chdir(prev); err != nil {
			tb.Fatalf("cannot revert working directory: %s", err)
		}
	})
}

func makeTerminalLoggerQuiet(t testing.TB) {
}
