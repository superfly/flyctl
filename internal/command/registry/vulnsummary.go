package registry

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/spinner"
	"github.com/superfly/flyctl/iostreams"
)

const (
	concurrentScans = 5
)

func newVulnSummary() *cobra.Command {
	const (
		usage = "vulnsummary <vulnid> ... [flags]"
		short = "Show a summary of possible vulnerabilities in registry images [experimental]"
		long  = "Summarize possible vulnerabilities in registry images in an org, by app.\n" +
			"Limit scanning to a single app if specified. Limit scanning to images\n" +
			"used by running machines if specified. Limit reporting to\n" +
			"specific vulnerability IDs or severities if specified."
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
			Description: "Only scan images for running machines",
		},
		flag.String{
			Name:        "severity",
			Shorthand:   "S",
			Description: fmt.Sprintf("Report only issues with a specific severity %v", allowedSeverities),
		},
	)

	return cmd
}

func runVulnSummary(ctx context.Context) error {
	var err error
	filter, err := argsGetVulnFilter(ctx)
	if err != nil {
		return err
	}

	imgs, err := argsGetImages(ctx)
	if err != nil {
		return err
	}

	imageScan, err := fetchImageScans(ctx, imgs, filter)
	if err != nil {
		return err
	}

	// calculate findings tables
	type AppPath struct {
		App  string
		Path string
	}
	allVids := map[string]bool{}
	vidsByApp := map[string]map[string]bool{}
	appImgsScanned := map[AppPath]bool{}
	for img := range imgs {
		scan := imageScan[img.Path]
		if scan == nil {
			continue
		}

		k := AppPath{img.App, img.Path}
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
	for _, img := range SortedKeys(imgs) {
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
	fmt.Fprintf(ios.Out, "To scan an image run: fly registry vulns -a <app> -i <imgpath>\n")
	fmt.Fprintf(ios.Out, "To download an SBOM run: fly registry sbom -a <app> -i <imgpath>\n")
	fmt.Fprintf(ios.Out, "\n")

	// Report checkmark table with columns of apps and rows of vulns.
	apps := lo.Keys(vidsByApp)
	slices.SortFunc(apps, strings.Compare)
	vids := lo.Keys(allVids)
	slices.SortFunc(vids, cmpVulnId)
	slices.Reverse(vids)

	var rows [][]string
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

// fetchImageScans returns a scan for each image path.
func fetchImageScans(ctx context.Context, imgs map[ImgInfo]Unit, filter *VulnFilter) (map[string]*Scan, error) {
	ios := iostreams.FromContext(ctx)
	spin := spinner.Run(ios, "Scanning...")
	defer spin.Stop()

	// Make all org tokens. Right now there will only be one.
	orgToken := make(map[string]string)
	for img := range imgs {
		if _, ok := orgToken[img.OrgID]; ok {
			continue
		}
		token, err := makeScantronToken(ctx, img.OrgID)
		if err != nil {
			return nil, err
		}
		orgToken[img.OrgID] = token
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(concurrentScans)
	mu := sync.Mutex{}
	imageScan := make(map[string]*Scan)
	skipped := make(map[ImgInfo]string)
	for img := range imgs {
		img := img
		mu.Lock()
		_, ok := imageScan[img.Path]
		if ok {
			mu.Unlock()
			continue
		}
		imageScan[img.Path] = nil
		token := orgToken[img.OrgID]
		mu.Unlock()

		eg.Go(func() error {
			scan, err := getVulnScan(ctx, img.Path, token)
			if err != nil {
				errUnsupportedPath := ErrUnsupportedPath("")
				var msg string
				if errors.As(err, &errUnsupportedPath) {
					msg = "from unsupported repository"
				} else {
					msg = fmt.Sprintf("error getting scan: %v", err)
				}
				mu.Lock()
				skipped[img] = msg
				mu.Unlock()
				return nil
			}

			scan = filterScan(scan, filter)
			mu.Lock()
			imageScan[img.Path] = scan
			mu.Unlock()
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	spin.Stop()

	for _, img := range SortedKeys(skipped) {
		msg := skipped[img]
		fmt.Fprintf(ios.Out, "Skipping %s (%s) %s\n", img.App, img.Mach, msg)
	}
	return imageScan, nil
}
