package launch

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
)

func TestManagedPostgresSecrets(t *testing.T) {
	for _, tt := range []struct {
		name     string
		endpoint flaps.ManagedPostgresEndpoint
		direct   string
		warning  string
	}{
		{
			name:     "both secrets",
			endpoint: flaps.ManagedPostgresEndpoint{Host: "direct.test", Port: 5433},
			direct:   "postgresql://fly-user:p%3A%2F%3F%23@direct.test:5433/fly-db",
		},
		{
			name:     "missing host",
			endpoint: flaps.ManagedPostgresEndpoint{Port: 5433},
			warning:  "Warning: DIRECT_URL was not set because the direct endpoint is missing or invalid.",
		},
		{
			name:     "missing port",
			endpoint: flaps.ManagedPostgresEndpoint{Host: "direct.test"},
			warning:  "Warning: DIRECT_URL was not set because the direct endpoint is missing or invalid.",
		},
		{
			name:     "invalid endpoint",
			endpoint: flaps.ManagedPostgresEndpoint{Host: "bad host", Port: 5433},
			warning:  "Warning: DIRECT_URL was not set because the direct endpoint is missing or invalid.",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var cluster flaps.ManagedPostgresCluster
			cluster.Endpoints.Primary.Pooler = flaps.ManagedPostgresEndpoint{Host: "pooler.test", Port: 5432}
			cluster.Endpoints.Primary.Direct = tt.endpoint

			secrets, warning := managedPostgresSecrets(cluster, "p:/?#")
			want := map[string]string{"DATABASE_URL": "postgresql://fly-user:p%3A%2F%3F%23@pooler.test/fly-db"}
			if tt.direct != "" {
				want["DIRECT_URL"] = tt.direct
			}
			require.Equal(t, want, secrets)
			require.Equal(t, tt.warning, warning)
		})
	}
}
