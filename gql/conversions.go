package gql

import "github.com/superfly/fly-go/api"

// AppForFlaps converts the genqclient AppFragment to an AppCompact suitable for flaps, which only needs two fields
func ToAppCompact(app AppData) *api.AppCompact {
	return &api.AppCompact{
		Name:            app.Name,
		Deployed:        app.Deployed,
		PlatformVersion: string(app.PlatformVersion),
		Organization: &api.OrganizationBasic{
			Slug: app.Organization.Slug,
		},
	}
}
