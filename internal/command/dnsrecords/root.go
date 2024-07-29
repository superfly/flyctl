package dnsrecords

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Manage DNS records"
		long  = "Manage DNS records within a domain"
	)
	cmd := command.New("dns-records", short, long, nil)
	cmd.Deprecated = "`fly dns-records` will be removed in a future release"
	cmd.Hidden = true
	cmd.AddCommand(
		newDNSRecordsList(),
		newDNSRecordsExport(),
		newDNSRecordsImport(),
	)
	return cmd
}

func newDNSRecordsList() *cobra.Command {
	const (
		short = `List DNS records`
		long  = `List DNS records within a domain`
	)
	cmd := command.New("list <domain>", short, long, runDNSRecordsList,
		command.RequireSession,
	)
	cmd.Deprecated = "`fly dns-records list` will be removed in a future release"
	flag.Add(cmd,
		flag.JSONOutput(),
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func newDNSRecordsExport() *cobra.Command {
	const (
		short = "Export DNS records"
		long  = `Export DNS records. Will write to a file if a filename is given, otherwise writers to StdOut.`
	)
	cmd := command.New("export <domain> [filename]", short, long, runDNSRecordsExport,
		command.RequireSession,
	)
	cmd.Deprecated = "`fly dns-records export` will be removed in a future release"
	cmd.Args = cobra.RangeArgs(1, 2)
	return cmd
}

func newDNSRecordsImport() *cobra.Command {
	const (
		short = "Import DNS records"
		long  = `Import DNS records. Will import from a file is a filename is given, otherwise imports from StdIn.`
	)
	cmd := command.New("import <domain> [filename]", short, long, runDNSRecordsImport,
		command.RequireSession,
	)
	cmd.Deprecated = "`fly dns-records import` will be removed in a future release"
	cmd.Args = cobra.RangeArgs(1, 2)
	return cmd
}

func runDNSRecordsList(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	name := flag.FirstArg(ctx)

	records, err := apiClient.GetDNSRecords(ctx, name)
	if err != nil {
		return err
	}

	fmt.Printf("Records for domain %s\n", name)

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, records)
		return nil
	}

	table := tablewriter.NewWriter(io.Out)
	table.SetAutoWrapText(true)
	table.SetReflowDuringAutoWrap(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetNoWhiteSpace(true)
	table.SetTablePadding(" ")
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeader([]string{"FQDN", "TTL", "Type", "Content"})

	for _, record := range records {
		table.Append([]string{record.FQDN, strconv.Itoa(record.TTL), record.Type, record.RData})
	}

	table.Render()

	return nil
}

func runDNSRecordsExport(ctx context.Context) error {
	name := flag.FirstArg(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	domain, err := apiClient.GetDomain(ctx, name)
	if err != nil {
		return err
	}

	records, err := apiClient.ExportDNSRecords(ctx, domain.ID)
	if err != nil {
		return err
	}

	args := flag.Args(ctx)
	if len(args) == 1 {
		fmt.Println(records)
	} else {
		filename := args[1]

		_, err := os.Stat(filename)
		if err == nil {
			return fmt.Errorf("File %s already exists", filename)
		}

		err = os.WriteFile(filename, []byte(records), 0o644)
		if err != nil {
			return err
		}

		fmt.Printf("Zone exported to %s\n", filename)
	}

	return nil
}

func runDNSRecordsImport(ctx context.Context) error {
	name := flag.FirstArg(ctx)
	apiClient := flyutil.ClientFromContext(ctx)

	var filename string

	args := flag.Args(ctx)
	if len(args) == 1 {
		// One arg, use stdin
		filename = "-"
	} else {
		filename = args[1]
	}

	domain, err := apiClient.GetDomain(ctx, name)
	if err != nil {
		return err
	}

	var data []byte

	if filename != "-" {
		data, err = os.ReadFile(filename)
		if err != nil {
			return err
		}
	} else {
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
	}

	warnings, changes, err := apiClient.ImportDNSRecords(ctx, domain.ID, string(data))
	if err != nil {
		return err
	}

	fmt.Printf("Zonefile import report for %s\n", domain.Name)

	if filename == "-" {
		fmt.Printf("Imported from stdin\n")
	} else {
		fmt.Printf("Imported from %s\n", filename)
	}

	fmt.Printf("%d warnings\n", len(warnings))
	for _, warning := range warnings {
		fmt.Println("->", warning.Action, warning.Message)
	}

	fmt.Printf("%d changes\n", len(changes))
	for _, change := range changes {
		switch change.Action {
		case "CREATE":
			fmt.Println("-> Created", change.NewText)
		case "DELETE":
			fmt.Println("-> Deleted", change.OldText)
		case "UPDATE":
			fmt.Println("-> Updated", change.OldText, "=>", change.NewText)
		}
	}

	return nil
}
