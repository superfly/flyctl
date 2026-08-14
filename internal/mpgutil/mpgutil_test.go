package mpgutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
)

func TestConnectionURI(t *testing.T) {
	var cluster flaps.ManagedPostgresCluster

	// A non-default port is kept, so the URI stays correct if the pooler ever
	// moves off 5432.
	cluster.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.test", Port: 5433}
	require.Equal(t, "postgresql://fly-user:s3cret@pooler.test:5433/fly-db", ConnectionURI(cluster, "s3cret"))

	// The default port is omitted, matching what the web UI shows.
	cluster.Endpoints.Primary.Pooler.Port = DefaultPort
	require.Equal(t, "postgresql://fly-user:s3cret@pooler.test/fly-db", ConnectionURI(cluster, "s3cret"))

	// The API omits endpoints until the cluster is ready, so a zero port is
	// treated as the default too.
	cluster.Endpoints.Primary.Pooler.Port = 0
	require.Equal(t, "postgresql://fly-user:s3cret@pooler.test/fly-db", ConnectionURI(cluster, "s3cret"))
}
