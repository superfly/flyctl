package registry

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/superfly/flyctl/internal/flag"
)

var allowedSeverities = []string{"LOW", "MEDIUM", "HIGH", "CRITICAL"}

type VulnFilter struct {
	SeverityLevel int
	VulnIds       map[string]bool
}

func (p *VulnFilter) IsSpecified() bool {
	return p.SeverityLevel > -1 && len(p.VulnIds) > 0
}

// argsGetVulnFilter returns a VulnFilter from command line args
// using `severity` and positional arguments.
func argsGetVulnFilter(ctx context.Context) (*VulnFilter, error) {
	vulnIds := flag.Args(ctx)
	sev := flag.GetString(ctx, "severity")
	if flag.IsSpecified(ctx, "severity") && !lo.Contains(allowedSeverities, sev) {
		return nil, fmt.Errorf("severity (%s) must be one of %v", sev, allowedSeverities)
	}

	f := &VulnFilter{
		SeverityLevel: severityLevel(sev),
	}

	if len(vulnIds) > 0 {
		f.VulnIds = make(map[string]bool)
		for _, vulnId := range vulnIds {
			f.VulnIds[vulnId] = true
		}
	}
	return f, nil
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
	if filter.VulnIds != nil {
		if !filter.VulnIds[vuln.VulnerabilityID] {
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
	var newRes []ScanResult
	for _, res := range scan.Results {
		var newVulns []ScanVuln
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
