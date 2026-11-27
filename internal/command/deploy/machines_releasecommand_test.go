package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/logs"
)

func TestReleaseCommandLogOptionsUsesMachineRegion(t *testing.T) {
	// The deployer runs on a Fly Machine in iad, so FLY_REGION=iad, while the
	// app (and its release command machine) live in arn.
	t.Setenv("FLY_REGION", "iad")
	opts := &logs.LogOptions{AppName: "my-app", RegionCode: "iad"}

	releaseCommandLogOptions(opts, &fly.Machine{ID: "d896311a553198", Region: "arn"})

	assert.Equal(t, "d896311a553198", opts.VMID)
	assert.Equal(t, "arn", opts.RegionCode)
}
