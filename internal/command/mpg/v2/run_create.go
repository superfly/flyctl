package cmdv2

import (
	"context"

	"github.com/superfly/fly-go"
)

type CreateClusterParams struct {
	Name           string
	OrgSlug        string
	Region         string
	Plan           string
	VolumeSizeGB   int
	PostGISEnabled bool
	PGMajorVersion int
}

func RunCreate(ctx context.Context, org *fly.Organization, appName string) error {
	return nil
}
