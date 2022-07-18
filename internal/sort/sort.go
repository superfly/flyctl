// Package sort implements common sorting functions.
package sort

import (
	"sort"

	"github.com/superfly/flyctl/api"
)

// OrganizationsByTypeAndName sorts orgs by their type and name.
func OrganizationsByTypeAndName(orgs []api.Organization) {
	sort.Slice(orgs, func(i, j int) bool {
		return orgs[i].Type < orgs[j].Type || orgs[i].Name < orgs[j].Name
	})
}

// RegionsByNameAndCode sorts regions by their name and code.
func RegionsByNameAndCode(regions []api.Region) {
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Name < regions[j].Name &&
			regions[i].Code < regions[j].Code
	})
}

// VMSizesByName sorts VM sizes by their name.
func VMSizesByName(sizes []api.VMSize) {
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i].Name < sizes[j].Name
	})
}
