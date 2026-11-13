package mpgutil

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/superfly/fly-go/flaps"
)

// Defaults for a Managed Postgres cluster.
const (
	DefaultUsername       = "fly-user"
	DefaultDatabase       = "fly-db"
	DefaultPort           = 5432
	DefaultPGMajorVersion = 17
)

// supportedPGMajorVersions lists the Postgres major versions a new cluster can
// run, oldest first.
var supportedPGMajorVersions = []int{16, 17, 18}

// SupportedPGMajorVersionsText lists the supported Postgres major versions for
// help and error text, as in "16, 17 and 18".
func SupportedPGMajorVersionsText() string {
	names := make([]string, len(supportedPGMajorVersions))
	for i, version := range supportedPGMajorVersions {
		names[i] = strconv.Itoa(version)
	}

	last := len(names) - 1

	return strings.Join(names[:last], ", ") + " and " + names[last]
}

// ValidatePGMajorVersion reports whether a new cluster can run the given
// Postgres major version.
func ValidatePGMajorVersion(version int) error {
	if slices.Contains(supportedPGMajorVersions, version) {
		return nil
	}

	return fmt.Errorf("invalid Postgres major version: %d. Supported versions are %s", version, SupportedPGMajorVersionsText())
}

// developerPlan is the key of the Developer plan.
const developerPlan = "dev-256"

// AppEndpoint returns the endpoint an app's DATABASE_URL points at: the direct
// one on the Developer plan (dev-256), the pooler on the others.
func AppEndpoint(c flaps.ManagedPostgresCluster) flaps.ManagedPostgresEndpoint {
	if c.Plan == developerPlan {
		return c.Endpoints.Primary.Direct
	}

	return c.Endpoints.Primary.Pooler
}

// ConnectionURI builds a libpq connection string for the cluster's default user
// and database via AppEndpoint, using the given password. The API returns no
// connection string, so flyctl assembles it client-side.
func ConnectionURI(c flaps.ManagedPostgresCluster, password string) string {
	endpoint := AppEndpoint(c)
	host := endpoint.Host
	if endpoint.Port != 0 && endpoint.Port != DefaultPort {
		host = fmt.Sprintf("%s:%d", host, endpoint.Port)
	}

	u := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(DefaultUsername, password),
		Host:   host,
		Path:   "/" + DefaultDatabase,
	}

	return u.String()
}
