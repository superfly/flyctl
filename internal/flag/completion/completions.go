package completion

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/flag/flagnames"
)

func CompleteApps(
	ctx context.Context,
	cmd *cobra.Command,
	args []string,
	partial string,
) ([]string, error) {

	var (
		client    = client.FromContext(ctx)
		clientApi = client.API()

		apps []api.App
		err  error
	)

	orgFiltered := false

	// We can't use `flag.*` here because of import cycles. *sigh*
	orgFlag := cmd.Flag(flagnames.Org)
	if orgFlag != nil && orgFlag.Changed {
		var org *api.Organization
		org, err = clientApi.GetOrganizationBySlug(ctx, orgFlag.Value.String())
		if err != nil {
			return nil, err
		}
		apps, err = clientApi.GetAppsForOrganization(ctx, org.ID)
		orgFiltered = true
	} else {
		apps, err = clientApi.GetApps(ctx, nil)
	}
	if err != nil {
		return nil, err
	}

	ret := lo.FilterMap(apps, func(app api.App, _ int) (string, bool) {
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

	clientApi := client.FromContext(ctx).API()

	format := func(org api.Organization) string {
		return fmt.Sprintf("%s\t%s", org.Slug, org.Name)
	}

	personal, others, err := clientApi.GetCurrentOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	names := []string{format(personal)}
	for _, org := range others {
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

	clientApi := client.FromContext(ctx).API()

	format := func(org api.Region) string {
		return fmt.Sprintf("%s\t%s", org.Code, org.Name)
	}

	// TODO(ali): Do we need to worry about which ones are marked as "gateway"?
	regions, reqRegion, err := clientApi.PlatformRegions(ctx)
	if err != nil {
		return nil, err
	}
	regionNames := lo.FilterMap(regions, func(region api.Region, _ int) (string, bool) {
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
