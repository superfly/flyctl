package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

var allowedSeverities = []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}

func newVulns() *cobra.Command {
	const (
		usage = "vulns <vulnid> ... [flags]"
		short = "Scan an application's image for vulnerabilities"
		long  = "Generate a text or JSON report of vulnerabilities found in a application's image.\n" +
			"If a machine is selected the image from that machine is scanned. Otherwise the image\n" +
			"of the first running machine is scanned. When a severity is specified, any vulnerabilities\n" +
			"less than the severity are omitted. When vulnIds are specified, any vulnerability not\n" +
			"in the vulnID list is omitted."
	)
	cmd := command.New(usage, short, long, runVulns,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ArbitraryArgs
	flag.Add(
		cmd,
		flag.App(),
		flag.String{
			Name:        "machine",
			Shorthand:   "m",
			Description: "Scan the image of the machine with the specified ID",
		},
		flag.Bool{
			Name:        "select",
			Shorthand:   "s",
			Description: "Select which machine to scan the image of from a list",
			Default:     false,
		},
		flag.String{
			Name:        "severity",
			Shorthand:   "S",
			Description: fmt.Sprintf("Report only issues with a specific severity %v", allowedSeverities),
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

type VulnFilter struct {
	SeverityLevel int
	VulnIds       []string
}

func runVulns(ctx context.Context) error {
	var (
		appName   = appconfig.NameFromContext(ctx)
		apiClient = flyutil.ClientFromContext(ctx)
	)

	vulnIds := flag.Args(ctx)
	sev := flag.GetString(ctx, "severity")
	if flag.IsSpecified(ctx, "severity") && !lo.Contains(allowedSeverities, sev) {
		return fmt.Errorf("severity (%s) must be one of %v", sev, allowedSeverities)
	}
	filter := &VulnFilter{severityLevel(sev), vulnIds}

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

	scan = filterScan(scan, filter)
	return presentScan(ctx, scan)
}

// severityLevel converts an `allowedSeverity` into an integer,
// where larger integers are more severe. Unknown severities
// result in `-1`.
func severityLevel(sev string) int {
	return lo.IndexOf(allowedSeverities, sev)
}

// filterVuln returns true if the user's command line preferences
// indicate interested in the vulnerability.
func filterVuln(vuln *ScanVuln, filter *VulnFilter) bool {
	if filter.SeverityLevel > severityLevel(vuln.Severity) {
		return false
	}
	if len(filter.VulnIds) > 0 {
		if !lo.Contains(filter.VulnIds, vuln.VulnerabilityID) {
			return false
		}
	}
	return true
}

// cmpVulnId compares a and b component by component.
// Pairs of components are compared numerically if they are both numeric.
func cmpVulnId(a, b string) int {
	as := strings.Split(a, "-")
	bs := strings.Split(b, "-")
	for n, ax := range as {
		if n >= len(bs) {
			return 1
		}
		bx := bs[n]

		an, aerr := strconv.ParseUint(ax, 10, 32)
		bn, berr := strconv.ParseUint(bx, 10, 32)
		d := 0
		if aerr == nil && berr == nil {
			d = int(an) - int(bn)
		} else {
			d = strings.Compare(ax, bx)
		}
		if d != 0 {
			return d
		}
	}

	if len(as) < len(bs) {
		return 1
	}
	return slices.Compare(as, bs)
}

// revCmpVuln compares vulns for sorting by highest severity and
// most recent vulnID first.
func revCmpVuln(a, b ScanVuln) int {
	if a.Severity != b.Severity {
		return -(severityLevel(a.Severity) - severityLevel(b.Severity))
	}
	if a.VulnerabilityID != b.VulnerabilityID {
		return -(cmpVulnId(a.VulnerabilityID, b.VulnerabilityID))
	}
	if a.PkgName != b.PkgName {
		return -(strings.Compare(a.PkgName, b.PkgName))
	}
	if a.InstalledVersion != b.InstalledVersion {
		return strings.Compare(a.InstalledVersion, b.InstalledVersion)
	}
	return 0
}

// filterScan filters each vuln in each result based on the command
// line preferences of the user. Any empty results are discarded.
func filterScan(scan *Scan, filter *VulnFilter) *Scan {
	newRes := []ScanResult{}
	for _, res := range scan.Results {
		newVulns := []ScanVuln{}
		for _, vuln := range res.Vulnerabilities {
			if filterVuln(&vuln, filter) {
				newVulns = append(newVulns, vuln)
			}
		}
		if len(newVulns) > 0 {
			slices.SortFunc(newVulns, revCmpVuln)
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
