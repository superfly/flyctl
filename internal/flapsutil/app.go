package flapsutil

import "github.com/superfly/fly-go/flaps"

// PostgresAppRole is the app role Flaps reports for Fly Postgres clusters.
const PostgresAppRole = "postgres_cluster"

// IsPostgresApp reports whether app is a Fly Postgres cluster.
func IsPostgresApp(app *flaps.App) bool {
	return app != nil && app.AppRole == PostgresAppRole
}

// DefaultNetwork is the name Flaps reports for an organization's default
// network. The web API and WireGuard tunnels identify that network by an
// empty name instead.
const DefaultNetwork = "default"

// NetworkName returns the app's network name in the form the web API and
// WireGuard tunnels expect: empty for the organization's default network,
// and the custom network's name otherwise.
func NetworkName(app *flaps.App) string {
	if app == nil || app.Network == DefaultNetwork {
		return ""
	}

	return app.Network
}
