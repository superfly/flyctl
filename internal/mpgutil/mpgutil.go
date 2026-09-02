package mpgutil

import (
	"fmt"
	"net/url"

	"github.com/superfly/fly-go/flaps"
)

// Defaults for a Managed Postgres cluster.
const (
	DefaultUsername       = "fly-user"
	DefaultDatabase       = "fly-db"
	DefaultPort           = 5432
	DefaultPGMajorVersion = "16"
)

// ConnectionURI builds a libpq connection string for the cluster's default user
// and database via the pooler endpoint, using the given password. The API
// returns no connection string, so flyctl assembles it client-side.
func ConnectionURI(c flaps.ManagedPostgresCluster, password string) string {
	host := c.Endpoints.Primary.Pooler.Host
	if port := c.Endpoints.Primary.Pooler.Port; port != 0 && port != DefaultPort {
		host = fmt.Sprintf("%s:%d", host, port)
	}

	u := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(DefaultUsername, password),
		Host:   host,
		Path:   "/" + DefaultDatabase,
	}

	return u.String()
}
