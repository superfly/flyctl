package mpg

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
)

func TestConnectionURI(t *testing.T) {
	var cluster flaps.ManagedPostgresCluster
	cluster.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.test", Port: 5433}

	require.Equal(t, "postgres://fly-user:s3cret@pooler.test:5433/fly-db", ConnectionURI(cluster, "s3cret"))

	// The API omits endpoints until the cluster is ready, so a zero port falls
	// back to the default.
	cluster.Endpoints.Primary.Pooler.Port = 0
	require.Equal(t, "postgres://fly-user:s3cret@pooler.test:5432/fly-db", ConnectionURI(cluster, "s3cret"))
}
