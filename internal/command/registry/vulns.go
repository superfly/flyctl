package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newVulns() *cobra.Command {
	const (
		usage = "vulns <vulnid> ... [flags]"
		short = "Report possible vulnerabilities in a registry image [experimental]"
		long  = "Report possible vulnerabilities in a registry image in JSON or text.\n" +
			"The image is selected by name, or the image of the app's first machine\n" +
			"is used unless interactive machine selection or machine ID is specified\n" +
			"Limit text reporting to specific vulnerabilitie IDs or severities if specified."
	)
	cmd := command.New(usage, short, long, runVulns,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ArbitraryArgs
	flag.Add(
		cmd,
		flag.App(),
		flag.Bool{
			Name:        "json",
			Description: "Output the scan results in JSON format",
		},
		flag.String{
			Name:        "image",
			Shorthand:   "i",
			Description: "Scan the repository image",
		},
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
	)

	return cmd
}

func runVulns(ctx context.Context) error {
	filter, err := argsGetVulnFilter(ctx)
	if err != nil {
		return err
	}

	if flag.IsSpecified(ctx, "json") && filter.IsSpecified() {
		return fmt.Errorf("filtering by severity or CVE is not supported when outputting JSON")
	}

	imgPath, orgId, err := argsGetImgPath(ctx)
	if err != nil {
		return err
	}

	token, err := makeScantronToken(ctx, orgId)
	if err != nil {
		return err
	}

	res, err := scantronVulnscanReq(ctx, imgPath, token)
	if err != nil {
		return err
	}
	defer res.Body.Close() // skipcq: GO-S2307

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed fetching scan data (status code %d)", res.StatusCode)
	}

	if flag.GetBool(ctx, "json") {
		ios := iostreams.FromContext(ctx)
		if _, err := io.Copy(ios.Out, res.Body); err != nil {
			return fmt.Errorf("failed to read scan results: %w", err)
		}
		return nil
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

func presentScan(ctx context.Context, scan *Scan) error {
	ios := iostreams.FromContext(ctx)

	fmt.Fprintf(ios.Out, "Report created at: %s\n", scan.CreatedAt)
	for _, res := range scan.Results {
		fmt.Fprintf(ios.Out, "Target %s: %s\n", res.Type, res.Target)
		for _, vuln := range res.Vulnerabilities {
			fmt.Fprintf(ios.Out, "  %s %s: %s %s\n", vuln.Severity, vuln.VulnerabilityID, vuln.PkgName, vuln.InstalledVersion)
		}
	}
	return nil
}
