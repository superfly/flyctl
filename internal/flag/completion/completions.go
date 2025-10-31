package completion

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
)

func CompleteApps(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	partial string,
) ([]string, error) {
	var (
		client = flyutil.ClientFromContext(ctx)

		apps []fly.App
		err  error
	)

	orgFiltered := false

	// We can't use `flag.*` here because of import cycles. *sigh*
	orgFlag := cmd.Flag(flagnames.Org)
	if orgFlag != nil && orgFlag.Changed {
		var org *fly.Organization
		org, err = client.GetOrganizationBySlug(ctx, orgFlag.Value.String())
		if err != nil {
			return nil, err
		}
		apps, err = client.GetAppsForOrganization(ctx, org.ID)
		orgFiltered = true
	} else {
		apps, err = client.GetApps(ctx, nil)
	}
	if err != nil {
		return nil, err
	}

	ret := lo.FilterMap(apps, func(app fly.App, _ int) (string, bool) {
		if strings.HasPrefix(app.Name, partial) {
			var info []string
			if !orgFiltered {
				info = append(info, app.Organization.Name)
			}
			info = append(info, app.Status)
			return fmt.Sprintf("%s\t%s", app.Name, strings.Join(info, ", ")), true
		}
		return "", false
	})
	slices.Sort(ret)
	return ret, nil
}

func CompleteOrgs(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	partial string,
) ([]string, error) {
	client := uiexutil.ClientFromContext(ctx)

	format := func(org uiex.Organization) string {
		return fmt.Sprintf("%s\t%s", org.Slug, org.Name)
	}

	orgs, err := client.ListOrganizations(ctx, false)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, org := range orgs {
		names = append(names, format(org))
	}
	ret := lo.Filter(names, func(name string, _ int) bool {
		return strings.HasPrefix(name, partial)
	})
	slices.Sort(ret)
	return ret, nil
}

func CompleteRegions(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	partial string,
) ([]string, error) {
	// cannot use flapsutil, would be an import cycle
	client, err := flaps.NewWithOptions(ctx, flaps.NewClientOpts{})
	if err != nil {
		return nil, err
	}

	format := func(org fly.Region) string {
		return fmt.Sprintf("%s\t%s", org.Code, org.Name)
	}

	regions, err := client.GetRegions(ctx)
	if err != nil {
		return nil, err
	}

	reqRegion, foundReqRegion := lo.Find(regions.Regions, func(r fly.Region) bool {
		return r.Code == regions.Nearest
	})

	// Filter out deprecated regions
	regions.Regions = lo.Filter(regions.Regions, func(r fly.Region, _ int) bool {
		return !r.Deprecated
	})

	regionNames := lo.FilterMap(regions.Regions, func(region fly.Region, _ int) (string, bool) {
		if strings.HasPrefix(region.Code, partial) {
			return format(region), true
		}
		return "", false
	})
	slices.Sort(regionNames)
	// If the region we're closest to is in the list, put it at the top
	if foundReqRegion && strings.HasPrefix(reqRegion.Code, partial) {
		idx := slices.Index(regionNames, format(reqRegion))
		// Should always be true because of the check above, but just to be safe...
		if idx >= 0 {
			regionNames = append([]string{regionNames[idx]}, append(regionNames[:idx], regionNames[idx+1:]...)...)
		}
	}
	return regionNames, nil
}
