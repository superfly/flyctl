package gql

import (
	"github.com/superfly/fly-go/flaps"
)

// AppForFlaps converts the genqclient AppFragment to an AppCompact suitable for flaps, which only needs two fields
func ToAppFlaps(app AppData) *flaps.App {
	return &flaps.App{
		Name: app.Name,
		Organization: flaps.AppOrganizationInfo{
			Slug: app.Organization.Slug,
		},
	}
}
