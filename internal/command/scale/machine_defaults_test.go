package scale

import (
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
)

func TestScaleCacheDiskPerProcessGroup(t *testing.T) {
	cfg := appconfig.NewConfig()
	cfg.Processes = map[string]string{"app": "serve", "worker": "work"}
	cfg.CacheDisks = []appconfig.CacheDisk{{Size: "1024mib", Path: "/cache", Processes: []string{"worker"}}}
	defaults := newDefaults(cfg, fly.Release{ImageRef: "alpine"}, nil, nil, "", false, &fly.MachineGuest{CPUs: 1})
	app, err := defaults.ToMachineConfig("app")
	require.NoError(t, err)
	require.Nil(t, app.CacheDrive)
	worker, err := defaults.ToMachineConfig("worker")
	require.NoError(t, err)
	require.Equal(t, &fly.MachineCacheDrive{SizeMB: 1024, Path: "/cache"}, worker.CacheDrive)
}
