package mpgutil

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

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

// DirectConnectionURI uses only the public endpoint and selected credentials.
func DirectConnectionURI(endpoint flaps.ManagedPostgresEndpoint, user, password, db string) (string, error) {
	if endpoint.Host == "" || endpoint.Port < 1 || endpoint.Port > 65535 ||
		strings.ContainsAny(endpoint.Host, "[]") || (strings.Contains(endpoint.Host, ":") && net.ParseIP(endpoint.Host) == nil) {
		return "", fmt.Errorf("invalid Postgres endpoint")
	}
	uri := (&url.URL{
		Scheme: "postgresql",
		Host:   net.JoinHostPort(endpoint.Host, strconv.Itoa(endpoint.Port)),
		User:   url.UserPassword(user, password),
		Path:   "/" + db,
	}).String()
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Hostname() != endpoint.Host || parsed.Port() != strconv.Itoa(endpoint.Port) {
		return "", fmt.Errorf("invalid Postgres endpoint")
	}

	return uri, nil
}
