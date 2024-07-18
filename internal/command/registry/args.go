package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/iostreams"
)

type Unit struct{}

// ImgInfo carries image information for a machine.
type ImgInfo struct {
	Org   string
	OrgID string
	App   string
	Mach  string
	Path  string
}

func (a ImgInfo) Compare(b ImgInfo) int {
	if d := strings.Compare(a.Org, b.Org); d != 0 {
		return d
	}
	if d := strings.Compare(a.OrgID, b.OrgID); d != 0 {
		return d
	}
	if d := strings.Compare(a.App, b.App); d != 0 {
		return d
	}
	if d := strings.Compare(a.Mach, b.Mach); d != 0 {
		return d
	}
	if d := strings.Compare(a.Path, b.Path); d != 0 {
		return d
	}
	return 0
}

// AugmentMap includes all of src into targ.
func AugmentMap[K comparable, V any](targ, src map[K]V) {
	for k, v := range src {
		targ[k] = v
	}
}

// SortedKeys returns the keys in a map in sorted order.
// Could be made generic.
func SortedKeys[V any](m map[ImgInfo]V) []ImgInfo {
	keys := lo.Keys(m)
	slices.SortFunc(keys, func(a, b ImgInfo) int { return a.Compare(b) })
	return keys
}

// argsGetAppCompact returns the AppCompact for the selected app, using `app`.
func argsGetAppCompact(ctx context.Context) (*fly.AppCompact, error) {
	appName := appconfig.NameFromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)
	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}
	return app, nil
}

func getFlapsClient(ctx context.Context, app *fly.AppCompact) (*flaps.Client, error) {
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create flaps client for app %s: %w", app.Name, err)
	}
	return flapsClient, nil
}

// argsGetMachine returns the selected machine, using `app`, `select` and `machine`.
func argsGetMachine(ctx context.Context, app *fly.AppCompact) (*fly.Machine, error) {
	if flag.IsSpecified(ctx, "machine") {
		if flag.IsSpecified(ctx, "select") {
			return nil, errors.New("--machine can't be used with -s/--select")
		}
		return argsGetMachineByID(ctx, app)
	}
	return argsSelectMachine(ctx, app)
}

// argsSelectMachine lets the user select a machine if there are multiple machines and
// the user specified "-s". Otherwise it returns the first machine for an app.
// Using `select`.
func argsSelectMachine(ctx context.Context, app *fly.AppCompact) (*fly.Machine, error) {
	anyMachine := !flag.GetBool(ctx, "select")

	flapsClient, err := getFlapsClient(ctx, app)
	if err != nil {
		return nil, err
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	var options []string
	for _, machine := range machines {
		imgPath := imageRefPath(&machine.ImageRef)
		options = append(options, fmt.Sprintf("%s: %s %s %s", machine.Region, machine.ID, machine.Name, imgPath))
	}

	if len(machines) == 0 {
		return nil, fmt.Errorf("no machines found")
	}

	if anyMachine || len(machines) == 1 {
		return machines[0], nil
	}

	index := 0
	if err := prompt.Select(ctx, &index, "Select a machine:", "", options...); err != nil {
		return nil, fmt.Errorf("failed to prompt for a machine: %w", err)
	}
	return machines[index], nil
}

// argsGetMachineByID returns an app's machine using the `machine` argument.
func argsGetMachineByID(ctx context.Context, app *fly.AppCompact) (*fly.Machine, error) {
	flapsClient, err := getFlapsClient(ctx, app)
	if err != nil {
		return nil, err
	}

	machineID := flag.GetString(ctx, "machine")
	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return nil, err
	}

	return machine, nil
}

// argsGetImgPath returns an image path and its OrgID from the command line or from a
// selected app machine, using `app`, `image`, `select`, and `machine`.
func argsGetImgPath(ctx context.Context) (string, string, error) {
	app, err := argsGetAppCompact(ctx)
	if err != nil {
		return "", "", err
	}

	if flag.IsSpecified(ctx, "image") {
		if flag.IsSpecified(ctx, "machine") || flag.IsSpecified(ctx, "select") {
			return "", "", fmt.Errorf("image option cannot be used with machine and select options")
		}

		path := flag.GetString(ctx, "image")
		return path, app.Organization.ID, nil
	}

	machine, err := argsGetMachine(ctx, app)
	if err != nil {
		return "", "", err
	}

	return imageRefPath(&machine.ImageRef), app.Organization.ID, nil
}

// argsGetImages returns a list of images in ImgInfo format from
// command line args or the environment, using `org`, `app`, `running`.
func argsGetImages(ctx context.Context) (map[ImgInfo]Unit, error) {
	ios := iostreams.FromContext(ctx)
	spin := spinner.Run(ios, "Finding images...")
	defer spin.Stop()

	if appName := flag.GetApp(ctx); appName != "" {
		return argsGetAppImages(ctx, appName)
	} else if orgName := flag.GetOrg(ctx); orgName != "" {
		return argsGetOrgImages(ctx, orgName)
	} else if appName := appconfig.NameFromContext(ctx); appName != "" {
		return argsGetAppImages(ctx, appName)
	}
	return nil, fmt.Errorf("No org or application specified")
}

// argsGetOrgImages returns a list of images for an org in ImgInfo format
// from `running`.
func argsGetOrgImages(ctx context.Context, orgName string) (map[ImgInfo]Unit, error) {
	client := flyutil.ClientFromContext(ctx)
	org, err := client.GetOrganizationBySlug(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get org %q: %w", orgName, err)
	}

	apps, err := client.GetAppsForOrganization(ctx, org.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps for %q: %w", orgName, err)
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(concurrentScans)
	mu := sync.Mutex{}
	allImgs := make(map[ImgInfo]Unit)
	for n := range apps {
		n := n
		eg.Go(func() error {
			app := &apps[n]
			imgs, err := argsGetOrgAppImages(ctx, org.Name, org.ID, app.Name)
			if err != nil {
				return fmt.Errorf("failed to fetch images for %q app: %w", app.Name, err)
			}

			mu.Lock()
			AugmentMap(allImgs, imgs)
			mu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return allImgs, nil
}

// argsGetAppImages returns a list of images for an app in ImgInfo format
// from `running`.
func argsGetAppImages(ctx context.Context, appName string) (map[ImgInfo]Unit, error) {
	apiClient := flyutil.ClientFromContext(ctx)
	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("failed to get app %q: %w", appName, err)
	}

	org := app.Organization
	return argsGetOrgAppImages(ctx, org.Name, org.ID, app.Name)
}

// argsGetOrgAppImages returns a list of images for an org/app in ImgInfo format
// from `running`.
func argsGetOrgAppImages(ctx context.Context, orgName, orgId, appName string) (map[ImgInfo]Unit, error) {
	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		OrgSlug: orgId,
		AppName: appName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create flaps client for %q: %w", appName, err)
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	if flag.GetBool(ctx, "running") {
		machines = lo.Filter(machines, func(machine *fly.Machine, _ int) bool {
			return machine.State == fly.MachineStateStarted
		})
	}

	imgs := make(map[ImgInfo]Unit)
	for _, machine := range machines {
		ir := machine.ImageRef
		imgPath := fmt.Sprintf("%s/%s@%s", ir.Registry, ir.Repository, ir.Digest)

		img := ImgInfo{
			Org:   orgName,
			OrgID: orgId,
			App:   appName,
			Mach:  machine.Name,
			Path:  imgPath,
		}
		imgs[img] = Unit{}
	}
	return imgs, nil
}
