package flapsutil

import "github.com/superfly/fly-go/flaps"

// PostgresAppRole is the app role Flaps reports for Fly Postgres clusters.
const PostgresAppRole = "postgres_cluster"

// IsPostgresApp reports whether app is a Fly Postgres cluster.
func IsPostgresApp(app *flaps.App) bool {
	return app != nil && app.AppRole == PostgresAppRole
}
