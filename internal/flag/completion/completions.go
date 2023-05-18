package completion

import (
	"context"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"golang.org/x/exp/slices"
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

	// We can't use `flag.*` here because of import cycles. *sigh*
	orgFlag := cmd.Flag("org")
	if orgFlag != nil && orgFlag.Changed {
		var org *api.Organization
		org, err = clientApi.GetOrganizationBySlug(ctx, orgFlag.Value.String())
		if err != nil {
			return nil, err
		}
		apps, err = clientApi.GetAppsForOrganization(ctx, org.ID)
	} else {
		apps, err = clientApi.GetApps(ctx, nil)
	}
	if err != nil {
		return nil, err
	}

	ret := lo.FilterMap(apps, func(app api.App, _ int) (string, bool) {
		if strings.HasPrefix(app.Name, partial) {
			return app.Name, true
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

	personal, others, err := clientApi.GetCurrentOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	names := []string{personal.Slug}
	for _, org := range others {
		names = append(names, org.Slug)
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

	// TODO(ali): Do we need to worry about which ones are marked as "gateway"?
	regions, reqRegion, err := clientApi.PlatformRegions(ctx)
	if err != nil {
		return nil, err
	}
	regionNames := lo.FilterMap(regions, func(region api.Region, _ int) (string, bool) {
		if strings.HasPrefix(region.Code, partial) {
			return region.Code, true
		}
		return "", false
	})
	slices.Sort(regionNames)
	// If the region we're closest to is in the list, put it at the top
	if reqRegion != nil && strings.HasPrefix(reqRegion.Code, partial) {
		idx := slices.Index(regionNames, reqRegion.Code)
		// Should always be true because of the check above, but just to be safe...
		if idx >= 0 {
			regionNames = append([]string{regionNames[idx]}, append(regionNames[:idx], regionNames[idx+1:]...)...)
		}
	}
	return regionNames, nil
}
