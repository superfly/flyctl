// Package sort implements common sorting functions.
package sort

import (
	"sort"

	"github.com/superfly/flyctl/api"
)

// OrganizationsByTypeAndName sorts orgs by their type and name.
func OrganizationsByTypeAndName(orgs []api.Organization) {
	sort.Slice(orgs, func(i, j int) bool {
		return orgs[i].Type < orgs[j].Type && orgs[i].Name < orgs[j].Name
	})
}
