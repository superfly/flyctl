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

func TestDirectConnectionURI(t *testing.T) {
	for _, tt := range []struct {
		name, host, want string
		port             int
	}{
		{name: "valid host", host: "direct.fly.dev", port: 5433, want: "postgresql://a%40b:p%3A%2F%3F%23@direct.fly.dev:5433/db"},
		{name: "IPv6 bracketed in URI", host: "fdaa::1", port: 5433, want: "postgresql://a%40b:p%3A%2F%3F%23@[fdaa::1]:5433/db"},
		{name: "empty host", port: 5433},
		{name: "bad port", host: "host", port: 65536},
		{name: "malformed host", host: "bad host", port: 5433},
		{name: "host includes port", host: "host:5433", port: 5433},
		{name: "bracketed input", host: "[::1]", port: 5433},
	} {
		t.Run(tt.name, func(t *testing.T) {
			uri, err := DirectConnectionURI(flaps.ManagedPostgresEndpoint{Host: tt.host, Port: tt.port}, "a@b", "p:/?#", "db")
			if tt.want == "" {
				require.EqualError(t, err, "invalid Postgres endpoint")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.want, uri)
		})
	}
}
