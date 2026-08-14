package mpg

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
	port := c.Endpoints.Primary.Pooler.Port
	if port == 0 {
		port = DefaultPort
	}

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(DefaultUsername, password),
		Host:   fmt.Sprintf("%s:%d", c.Endpoints.Primary.Pooler.Host, port),
		Path:   "/" + DefaultDatabase,
	}

	return u.String()
}
