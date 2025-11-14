package gql

import (
	fly "github.com/superfly/fly-go"
)

// AppForFlaps converts the genqclient AppFragment to an AppCompact suitable for flaps, which only needs two fields
func ToAppCompact(app AppData) *fly.AppCompact {
	return &fly.AppCompact{
		Name:            app.Name,
		Deployed:        app.Deployed,
		PlatformVersion: string(app.PlatformVersion),
		Organization: &fly.OrganizationBasic{
			Slug: app.Organization.Slug,
		},
	}
}
