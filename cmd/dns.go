package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
)

func newDNSCommand(client *client.Client) *Command {
	dnsStrings := docstrings.Get("dns-records")
	cmd := BuildCommandKS(nil, nil, dnsStrings, client, requireSession)

	listStrings := docstrings.Get("dns-records.list")
	listCmd := BuildCommandKS(cmd, runRecordsList, listStrings, client, requireSession)
	listCmd.Args = cobra.ExactArgs(1)

	recordsExportStrings := docstrings.Get("dns-records.export")
	recordsExportCmd := BuildCommandKS(cmd, runRecordsExport, recordsExportStrings, client, requireSession)
	recordsExportCmd.Args = cobra.MinimumNArgs(1)
	recordsExportCmd.Args = cobra.MaximumNArgs(3)
	recordsExportCmd.AddBoolFlag(BoolFlagOpts{
		Name: "overwrite",
	})

	recordsImportStrings := docstrings.Get("dns-records.import")
	recordsImportCmd := BuildCommandKS(cmd, runRecordsImport, recordsImportStrings, client, requireSession)
	recordsImportCmd.Args = cobra.MaximumNArgs(3)
	recordsImportCmd.Args = cobra.MinimumNArgs(1)

	return cmd
}

func runRecordsList(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	name := cmdCtx.Args[0]

	records, err := cmdCtx.Client.API().GetDNSRecords(ctx, name)
	if err != nil {
		return err
	}

	fmt.Printf("Records for domain %s\n", name)

	if cmdCtx.OutputJSON() {
		cmdCtx.WriteJSON(records)
		return nil
	}

	table := tablewriter.NewWriter(cmdCtx.Out)
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

func runRecordsExport(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	name := cmdCtx.Args[0]

	domain, err := cmdCtx.Client.API().GetDomain(ctx, name)
	if err != nil {
		return err
	}

	records, err := cmdCtx.Client.API().ExportDNSRecords(ctx, domain.ID)
	if err != nil {
		return err
	}

	if len(cmdCtx.Args) == 1 {
		fmt.Println(records)
	} else {
		var filename = cmdCtx.Args[1]

		_, err := os.Stat(filename)
		if err == nil {
			return fmt.Errorf("File %s already exists", filename)
		}

		err = ioutil.WriteFile(filename, []byte(records), 0644)
		if err != nil {
			return err
		}

		fmt.Printf("Zone exported to %s\n", filename)
	}

	return nil
}

func runRecordsImport(cmdCtx *cmdctx.CmdContext) error {
	ctx := createCancellableContext()

	name := cmdCtx.Args[0]
	var filename string

	if len(cmdCtx.Args) == 1 {
		// One arg, use stdin
		filename = "-"
	} else {
		filename = cmdCtx.Args[1]
	}

	domain, err := cmdCtx.Client.API().GetDomain(ctx, name)
	if err != nil {
		return err
	}

	var data []byte

	if filename != "-" {
		data, err = ioutil.ReadFile(filename)
		if err != nil {
			return err
		}
	} else {
		data, err = ioutil.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
	}

	warnings, changes, err := cmdCtx.Client.API().ImportDNSRecords(ctx, domain.ID, string(data))
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
