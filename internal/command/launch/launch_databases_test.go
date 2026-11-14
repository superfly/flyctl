package launch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

// fly launch has no flag for the Postgres version, so the request it sends
// decides what every launched cluster runs.
func TestManagedPostgresClusterRequest(t *testing.T) {
	got := managedPostgresClusterRequest("my-org", &plan.ManagedPostgresPlan{
		DbName:   "db",
		Region:   "iad",
		Plan:     "basic",
		DiskSize: 10,
	})

	assert.Equal(t, flaps.CreateManagedPostgresClusterRequest{
		OrgSlug:        "my-org",
		Name:           "db",
		Region:         "iad",
		Plan:           "basic",
		DiskSizeGB:     10,
		PGMajorVersion: "17",
	}, got)
}
