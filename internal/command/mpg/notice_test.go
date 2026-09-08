package mpg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/uiex/mpg"
	"github.com/superfly/flyctl/iostreams"
)

func TestPrintV2MigrationNotice(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)

	printV2MigrationNotice(ctx)

	assert.Contains(t, stderr.String(), "Managed Postgres v2 is here!")
	assert.Contains(t, stderr.String(), dashboardURL)
	assert.Empty(t, stdout.String())
}

func TestPrintV1MigrationLink(t *testing.T) {
	t.Run("v1 cluster gets its migration link", func(t *testing.T) {
		ios, _, _, stderr := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)

		printV1MigrationLink(ctx, &mpg.Cluster{
			Id:           "abc123",
			Name:         "my-db",
			Version:      mpg.VersionV1,
			Organization: fly.Organization{Slug: "acme"},
		}, "")

		assert.Contains(t, stderr.String(), `Cluster "my-db" is on MPG v1`)
		assert.Contains(t, stderr.String(), "https://fly.io/dashboard/acme/managed_postgres/abc123/v2-migration")
	})

	t.Run("falls back to the provided org slug", func(t *testing.T) {
		ios, _, _, stderr := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)

		printV1MigrationLink(ctx, &mpg.Cluster{
			Id:      "abc123",
			Name:    "my-db",
			Version: mpg.VersionV1,
		}, "personal")

		assert.Contains(t, stderr.String(), "https://fly.io/dashboard/personal/managed_postgres/abc123/v2-migration")
	})

	t.Run("v2 cluster prints nothing", func(t *testing.T) {
		ios, _, _, stderr := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)

		printV1MigrationLink(ctx, &mpg.Cluster{
			Id:      "abc123",
			Name:    "my-db",
			Version: mpg.VersionV2,
		}, "acme")

		assert.Empty(t, stderr.String())
	})

	t.Run("ineligible v1 cluster prints nothing", func(t *testing.T) {
		ios, _, _, stderr := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)
		eligible := false

		printV1MigrationLink(ctx, &mpg.Cluster{
			Id:                     "abc123",
			Name:                   "my-db",
			Version:                mpg.VersionV1,
			EligibleForV2Migration: &eligible,
		}, "acme")

		assert.Empty(t, stderr.String())
	})

	t.Run("eligible v1 cluster gets its migration link", func(t *testing.T) {
		ios, _, _, stderr := iostreams.Test()
		ctx := iostreams.NewContext(context.Background(), ios)
		eligible := true

		printV1MigrationLink(ctx, &mpg.Cluster{
			Id:                     "abc123",
			Name:                   "my-db",
			Version:                mpg.VersionV1,
			EligibleForV2Migration: &eligible,
		}, "acme")

		assert.Contains(t, stderr.String(), "https://fly.io/dashboard/acme/managed_postgres/abc123/v2-migration")
	})
}
