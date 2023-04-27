//go:build integration
// +build integration

package preflight

import (
	"testing"

	//"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	//"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestFlyDeploy_case01(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	appName := f.CreateRandomAppName()

	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --force-machines --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)
	f.Fly("scale count 1 --region %s --yes", f.SecondaryRegion())

	f.WriteFlyToml(`%s
[mounts]
	source = "data"
	destination = "/data"
	`, f.ReadFile("fly.toml"))

	x := f.FlyAllowExitFailure("deploy")
	require.Contains(f, x.StdErr().String(), `needs volumes with name 'data' to fullfill mounts defined in fly.toml`)

	// Create two volumes because fly launch will start 2 machines because of HA setup
	f.Fly("volume create -a %s -r %s -s 1 data -y", appName, f.PrimaryRegion())
	f.Fly("volume create -a %s -r %s -s 1 data -y", appName, f.SecondaryRegion())
	f.Fly("deploy")
}
