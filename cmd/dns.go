package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
)

func newDNSCommand() *Command {
	dnsStrings := docstrings.Get("dns-records")
	cmd := BuildCommandKS(nil, nil, dnsStrings, os.Stdout, requireSession)

	listStrings := docstrings.Get("dns-records.list")
	listCmd := BuildCommandKS(cmd, runRecordsList, listStrings, os.Stdout, requireSession)
	listCmd.Args = cobra.ExactArgs(1)

	recordsExportStrings := docstrings.Get("dns-records.export")
	recordsExportCmd := BuildCommandKS(cmd, runRecordsExport, recordsExportStrings, os.Stdout, requireSession)
	recordsExportCmd.Args = cobra.MinimumNArgs(1)
	recordsExportCmd.Args = cobra.MaximumNArgs(2)

	recordsImportStrings := docstrings.Get("dns-records.import")
	recordsImportCmd := BuildCommandKS(cmd, runRecordsImport, recordsImportStrings, os.Stdout, requireSession)
	recordsImportCmd.Args = cobra.MaximumNArgs(3)

	return cmd
}

func runRecordsList(ctx *cmdctx.CmdContext) error {
	name := ctx.Args[0]

	records, err := ctx.Client.API().GetDNSRecords(name)
	if err != nil {
		return err
	}

	fmt.Printf("Records for domain %s\n", name)

	if ctx.OutputJSON() {
		ctx.WriteJSON(records)
		return nil
	}

	table := tablewriter.NewWriter(ctx.Out)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{"FQDN", "TTL", "Type", "Content"})

	for _, record := range records {
		table.Append([]string{record.FQDN, strconv.Itoa(record.TTL), record.Type, record.RData})
	}

	table.Render()

	return nil
}

func runRecordsExport(ctx *cmdctx.CmdContext) error {
	name := ctx.Args[0]

	domain, err := ctx.Client.API().GetDomain(name)
	if err != nil {
		return err
	}

	records, err := ctx.Client.API().ExportDNSRecords(domain.ID)
	if err != nil {
		return err
	}

	if len(ctx.Args) == 1 {
		fmt.Println(records)
	} else {
		var filename string

		validateFile := func(val interface{}) error {
			_, err := os.Stat(val.(string))
			if err == nil {
				return fmt.Errorf("File %s already exists", val.(string))
			}
			return nil
		}

		err := survey.AskOne(&survey.Input{Message: "Export filename:"}, &filename, survey.WithValidator(validateFile))

		if err != nil {
			return err
		}

		ioutil.WriteFile(filename, []byte(records), 0644)

		fmt.Printf("Zone exported to %s\n", filename)
	}

	return nil
}

func runRecordsImport(ctx *cmdctx.CmdContext) error {
	name := ctx.Args[0]
	filename := ctx.Args[1]

	domain, err := ctx.Client.API().GetDomain(name)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	warnings, changes, err := ctx.Client.API().ImportDNSRecords(domain.ID, string(data))
	if err != nil {
		return err
	}

	fmt.Println("Zonefile import report")

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
