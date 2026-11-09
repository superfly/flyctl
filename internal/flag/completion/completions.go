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
	"github.com/superfly/flyctl/internal/flapsutil"
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
	type appInfo struct {
		name, orgName, status string
	}

	var apps []appInfo

	orgFiltered := false

	// We can't use `flag.*` here because of import cycles. *sigh*
	orgFlag := cmd.Flag(flagnames.Org)
	if orgFlag != nil && orgFlag.Changed {
		flapsClient := flapsutil.ClientFromContext(ctx)
		flapsApps, err := flapsClient.ListApps(ctx, flaps.ListAppsRequest{OrgSlug: orgFlag.Value.String()})
		if err != nil {
			return nil, err
		}
		for _, app := range flapsApps {
			apps = append(apps, appInfo{name: app.Name, orgName: app.Organization.Name, status: app.Status})
		}
		orgFiltered = true
	} else {
		client := flyutil.ClientFromContext(ctx)
		gqlApps, err := client.GetApps(ctx, nil)
		if err != nil {
			return nil, err
		}
		for _, app := range gqlApps {
			apps = append(apps, appInfo{name: app.Name, orgName: app.Organization.Name, status: app.Status})
		}
	}

	ret := lo.FilterMap(apps, func(app appInfo, _ int) (string, bool) {
		if strings.HasPrefix(app.name, partial) {
			var info []string
			if !orgFiltered {
				info = append(info, app.orgName)
			}
			info = append(info, app.status)

			return fmt.Sprintf("%s\t%s", app.name, strings.Join(info, ", ")), true
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
	format := func(org uiex.Organization) string {
		return fmt.Sprintf("%s\t%s", org.Slug, org.Name)
	}

	orgs, err := uiexutil.ClientFromContext(ctx).ListOrganizations(ctx, false)
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
	flapsClient := flapsutil.ClientFromContext(ctx)

	format := func(org fly.Region) string {
		return fmt.Sprintf("%s\t%s", org.Code, org.Name)
	}

	// TODO(ali): Do we need to worry about which ones are marked as "gateway"?
	regionData, err := flapsClient.GetRegions(ctx)
	if err != nil {
		return nil, err
	}
	regions := regionData.Regions

	var reqRegion *fly.Region
	if nearest, ok := lo.Find(regions, func(r fly.Region) bool { return r.Code == regionData.Nearest }); ok {
		reqRegion = &nearest
	}

	// Filter out deprecated regions
	regions = lo.Filter(regions, func(r fly.Region, _ int) bool {
		return !r.Deprecated
	})

	regionNames := lo.FilterMap(regions, func(region fly.Region, _ int) (string, bool) {
		if strings.HasPrefix(region.Code, partial) {
			return format(region), true
		}

		return "", false
	})
	slices.Sort(regionNames)
	// If the region we're closest to is in the list, put it at the top
	if reqRegion != nil && strings.HasPrefix(reqRegion.Code, partial) {
		idx := slices.Index(regionNames, format(*reqRegion))
		// Should always be true because of the check above, but just to be safe...
		if idx >= 0 {
			regionNames = append([]string{regionNames[idx]}, append(regionNames[:idx], regionNames[idx+1:]...)...)
		}
	}

	return regionNames, nil
}
