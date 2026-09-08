// Package sort implements common sorting functions.
package sort

import (
	"sort"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/uiex"
)

// OrganizationsByTypeAndName sorts orgs by their type and name.
func OrganizationsByTypeAndName(orgs []uiex.Organization) {
	sort.Slice(orgs, func(i, j int) bool {
		if orgs[i].Personal != orgs[j].Personal {
			return orgs[i].Personal
		}

		return orgs[i].Name < orgs[j].Name
	})
}

// RegionsByNameAndCode sorts regions by their name and code.
func RegionsByNameAndCode(regions []fly.Region) {
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].Name < regions[j].Name &&
			regions[i].Code < regions[j].Code
	})
}

// VMSizesBySize sorts VM sizes by their name.
func VMSizesBySize(sizes []fly.VMSize) {
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i].CPUCores < sizes[j].CPUCores
	})
}
