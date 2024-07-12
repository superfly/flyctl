package scan

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

// TODO: placeholder name.. figure out proper name for this one.
func newVulnSummary() *cobra.Command {
	const (
		usage = "vulnsummary <vulnid> ... [flags]"
		short = "Show vulnerabilities in machine images"
		long  = "XXX todo. vulns in images across apps and machines, for running apps, or even stopped apps."
	)
	cmd := command.New(usage, short, long, runVulnSummary,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.ArbitraryArgs
	flag.Add(
		cmd,
		flag.App(),
		flag.Org(),
		flag.Bool{
			Name:        "running",
			Shorthand:   "r",
			Description: "Only scan images for machines that are running, otherwise scan stopped machines as well.",
		},
		flag.String{
			Name:        "severity",
			Shorthand:   "S",
			Description: fmt.Sprintf("Report only issues with a specific severity %v", allowedSeverities),
		},
	)

	return cmd
}

// ImgInfo carries image information for a machine.
type ImgInfo struct {
	Org   string
	OrgID string
	App   string
	AppID string
	Mach  string
	Path  string
}

func is400(err error) bool {
	// Oh gross hack of all gross hacks.
	// TODO: do better
	return strings.Contains(err.Error(), "status code 400")
}

func runVulnSummary(ctx context.Context) error {
	var err error
	filter, err := getVulnFilter(ctx)
	if err != nil {
		return err
	}

	// enumerate all images of interest.
	var imgs []ImgInfo
	if appName := flag.GetApp(ctx); appName != "" {
		imgs, err = getAppImages(ctx, appName)
	} else if orgName := flag.GetOrg(ctx); orgName != "" {
		imgs, err = getOrgImages(ctx, orgName)
	} else if appName := appconfig.NameFromContext(ctx); appName != "" {
		imgs, err = getAppImages(ctx, appName)
	} else {
		err = fmt.Errorf("No org or application specified")
	}
	if err != nil {
		return err
	}

	// fetch all image scans.
	imageScan := map[string]*Scan{}
	token := ""
	tokenAppID := ""
	for _, img := range imgs {
		if _, ok := imageScan[img.Path]; ok {
			continue
		}

		if img.AppID != tokenAppID {
			tokenAppID = img.AppID
			token, err = makeScantronToken(ctx, img.OrgID, img.AppID)
			if err != nil {
				return err
			}
		}

		scan, err := getVulnScan(ctx, img.Path, token)
		if err != nil {
			if is400(err) {
				// TODO: not fmt.Printf, do better.
				fmt.Printf("Skipping %s (%s) from unsupported repository\n", img.App, img.Mach)
				continue
			}
			return fmt.Errorf("Getting vulnerability scan for %s (%s): %w", img.App, img.Mach, err)
		}
		imageScan[img.Path] = filterScan(scan, filter)
	}

	// calculate findings tables
	allVids := map[string]bool{}
	vidsByApp := map[string]map[string]bool{}
	appImgsScanned := map[string]bool{}
	for _, img := range imgs {
		scan := imageScan[img.Path]
		if scan == nil {
			continue
		}

		k := fmt.Sprintf("%s/%s", img.AppID, img.Path)
		if _, ok := appImgsScanned[k]; ok {
			continue
		}
		appImgsScanned[k] = true

		if _, ok := vidsByApp[img.App]; !ok {
			vidsByApp[img.App] = map[string]bool{}
		}
		appVids := vidsByApp[img.App]

		for _, res := range scan.Results {
			for _, vuln := range res.Vulnerabilities {
				vid := vuln.VulnerabilityID
				allVids[vid] = true
				appVids[vid] = true
			}
		}
	}

	// Show what is being scanned.
	ios := iostreams.FromContext(ctx)
	lastOrg := ""
	lastApp := ""
	fmt.Fprintf(ios.Out, "Scanned images\n")
	for _, img := range imgs {
		scan := imageScan[img.Path]
		if img.Org != lastOrg {
			fmt.Fprintf(ios.Out, "Org: %s\n", img.Org)
			lastOrg = img.Org
		}
		if img.App != lastApp {
			fmt.Fprintf(ios.Out, "  App: %s\n", img.App)
			lastApp = img.App
		}
		if scan != nil {
			fmt.Fprintf(ios.Out, "    %s\t%s\n", img.Mach, img.Path)
		} else {
			fmt.Fprintf(ios.Out, "    %s\t%s [skipped]\n", img.Mach, img.Path)
		}
	}
	fmt.Fprintf(ios.Out, "\n")
	fmt.Fprintf(ios.Out, "To scan an image run: flyctl image scan <imgpath>\n")
	fmt.Fprintf(ios.Out, "\n")

	// Report checkmark table with columns of apps and rows of vulns.
	// TODO: use flyctl table stuff for pretty pretty
	apps := lo.Keys(vidsByApp)
	slices.SortFunc(apps, strings.Compare)
	vids := lo.Keys(allVids)
	slices.SortFunc(vids, cmpVulnId)

	rows := [][]string{}
	for _, vid := range vids {
		row := []string{vid}
		for _, app := range apps {
			check := lo.Ternary(vidsByApp[app][vid], "X", "-")
			row = append(row, check)
		}
		rows = append(rows, row)
	}
	cols := append([]string{""}, apps...)
	render.Table(ios.Out, "Vulnerabilities in Apps", rows, cols...)

	return nil
}

func getOrgImages(ctx context.Context, orgName string) ([]ImgInfo, error) {
	client := flyutil.ClientFromContext(ctx)
	org, err := client.GetOrganizationBySlug(ctx, orgName)
	if err != nil {
		return nil, err
	}

	apps, err := client.GetAppsForOrganization(ctx, org.ID)
	if err != nil {
		return nil, err
	}

	allImgs := []ImgInfo{}
	for _, app := range apps {
		imgs, err := getAppImages(ctx, app.Name)
		if err != nil {
			return nil, fmt.Errorf("could not fetch images for %q app: %w", app.Name, err)
		}
		allImgs = append(allImgs, imgs...)
	}
	return allImgs, nil

}

func getAppImages(ctx context.Context, appName string) ([]ImgInfo, error) {
	apiClient := flyutil.ClientFromContext(ctx)
	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("failed to get app %q: %w", appName, err)
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create flaps client for %q: %w", appName, err)
	}
	org := app.Organization
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	if flag.GetBool(ctx, "running") {
		machines = lo.Filter(machines, func(machine *fly.Machine, _ int) bool {
			return machine.State == fly.MachineStateStarted
		})
	}

	imgs := []ImgInfo{}
	for _, machine := range machines {
		ir := machine.ImageRef
		imgPath := fmt.Sprintf("%s/%s@%s", ir.Registry, ir.Repository, ir.Digest)

		img := ImgInfo{
			Org:   org.Name,
			OrgID: org.ID,
			App:   app.Name,
			AppID: app.ID,
			Mach:  machine.Name,
			Path:  imgPath,
		}
		imgs = append(imgs, img)
	}
	return imgs, nil
}
