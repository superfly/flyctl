package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func newVulns() *cobra.Command {
	const (
		usage = "vulns"
		short = "Scan an application's image for vulnerabilities"
		long  = "Generate a text or JSON report of vulnerabilities found in a application's image.\n" +
			"If a machine is selected the image from that machine is scanned. Otherwise the image\n" +
			"of the first running machine is scanned."
	)
	cmd := command.New(usage, short, long, runVulns,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.NoArgs
	flag.Add(
		cmd,
		flag.App(),
		flag.String{
			Name:        "machine",
			Description: "Scan the image of the machine with the specified ID",
		},
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Description: "Select which machine to scan the image of from a list.",
			Default:     false,
		},
		// TODO: output file
		// TODO: format json/text
	)

	return cmd
}

type Scan struct {
	SchemaVersion int
	CreatedAt     string
	// Metadata
	Results []ScanResult
}

type ScanResult struct {
	Target          string
	Type            string
	Vulnerabilities []ScanVuln
}

type ScanVuln struct {
	VulnerabilityID  string
	PkgName          string
	InstalledVersion string
	Status           string
	Title            string
	Description      string
	Severity         string
}

func runVulns(ctx context.Context) error {
	var (
		appName   = appconfig.NameFromContext(ctx)
		apiClient = flyutil.ClientFromContext(ctx)
	)

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get app: %w", err)
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppCompact: app,
		AppName:    app.Name,
	})
	if err != nil {
		return fmt.Errorf("failed to create flaps client: %w", err)
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	machine, err := selectMachine(ctx, app)
	if err != nil {
		return err
	}

	res, err := scantron(ctx, apiClient, app, machine, false)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed fetching scan data (status code %d)", res.StatusCode)
	}

	scan := &Scan{}
	if err = json.NewDecoder(res.Body).Decode(scan); err != nil {
		return fmt.Errorf("failed to read scan results: %w", err)
	}
	if scan.SchemaVersion != 2 {
		return fmt.Errorf("scan result has the wrong schema")
	}

	scan = filterScan(ctx, scan)
	return presentScan(ctx, scan)
}

// filterVuln returns true if the user's command line preferences
// indicate interested in the vulnerability.
func filterVuln(ctx context.Context, vuln *ScanVuln) bool {
	// TODO: placeholder
	return vuln.VulnerabilityID == "CVE-2024-24791" ||
		vuln.VulnerabilityID == "CVE-2023-31484" ||
		vuln.Severity == "CRITICAL"
}

// filterScan filters each vuln in each result based on the command
// line preferences of the user. Any empty results are discarded.
func filterScan(ctx context.Context, scan *Scan) *Scan {
	newRes := []ScanResult{}
	for _, res := range scan.Results {
		newVulns := []ScanVuln{}
		for _, vuln := range res.Vulnerabilities {
			if filterVuln(ctx, &vuln) {
				newVulns = append(newVulns, vuln)
			}
		}
		if len(newVulns) > 0 {
			res.Vulnerabilities = newVulns
			newRes = append(newRes, res)
		}
	}
	scan.Results = newRes
	return scan
}

func presentScan(ctx context.Context, scan *Scan) error {
	ios := iostreams.FromContext(ctx)

	// TODO: scan.Metadata?
	fmt.Fprintf(ios.Out, "Report created at: %s\n", scan.CreatedAt)
	for _, res := range scan.Results {
		fmt.Fprintf(ios.Out, "Target %s: %s\n", res.Type, res.Target)
		for _, vuln := range res.Vulnerabilities {
			fmt.Fprintf(ios.Out, "  %s %s: %s %s\n", vuln.Severity, vuln.VulnerabilityID, vuln.PkgName, vuln.InstalledVersion)
		}
	}
	return nil
}
