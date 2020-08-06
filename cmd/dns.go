package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
)

func newDnsCommand() *Command {
	dnsStrings := docstrings.Get("dns")
	cmd := &Command{
		Command: &cobra.Command{
			Use:   dnsStrings.Usage,
			Short: dnsStrings.Short,
			Long:  dnsStrings.Long,
		},
	}

	zonesStrings := docstrings.Get("dns.zones")
	zonesCmd := &Command{
		Command: &cobra.Command{
			Use:   zonesStrings.Usage,
			Short: zonesStrings.Short,
			Long:  zonesStrings.Long,
		},
	}
	cmd.AddCommand(zonesCmd)

	zonesListStrings := docstrings.Get("dns.zones.list")
	zonesListCmd := BuildCommand(zonesCmd, runZonesList, zonesListStrings.Usage, zonesListStrings.Short, zonesListStrings.Long, os.Stdout, requireSession)
	zonesListCmd.Args = cobra.ExactArgs(1)

	zonesCreateStrings := docstrings.Get("dns.zones.create")
	zonesCreateCmd := BuildCommand(zonesCmd, runZonesCreate, zonesCreateStrings.Usage, zonesCreateStrings.Short, zonesCreateStrings.Long, os.Stdout, requireSession)
	zonesCreateCmd.Args = cobra.ExactArgs(2)

	zonesDeleteStrings := docstrings.Get("dns.zones.delete")
	zonesDeleteCmd := BuildCommand(zonesCmd, runZonesDelete, zonesDeleteStrings.Usage, zonesDeleteStrings.Short, zonesDeleteStrings.Long, os.Stdout, requireSession)
	zonesDeleteCmd.Args = cobra.ExactArgs(2)

	recordsStrings := docstrings.Get("dns.records")
	recordsCmd := &Command{
		Command: &cobra.Command{
			Use:   recordsStrings.Usage,
			Short: recordsStrings.Short,
			Long:  recordsStrings.Long,
		},
	}
	cmd.AddCommand(recordsCmd)

	recordsListStrings := docstrings.Get("dns.records.list")
	recordsListCmd := BuildCommand(recordsCmd, runRecordsList, recordsListStrings.Usage, recordsListStrings.Short, recordsListStrings.Long, os.Stdout, requireSession)
	recordsListCmd.Args = cobra.ExactArgs(2)

	recordsExportStrings := docstrings.Get("dns.records.export")
	recordsExportCmd := BuildCommand(recordsCmd, runRecordsExport, recordsExportStrings.Usage, recordsExportStrings.Short, recordsExportStrings.Long, os.Stdout, requireSession)
	recordsExportCmd.Args = cobra.ExactArgs(2)

	recordsImportStrings := docstrings.Get("dns.records.import")
	recordsImportCmd := BuildCommand(recordsCmd, runRecordsImport, recordsImportStrings.Usage, recordsImportStrings.Short, recordsImportStrings.Long, os.Stdout, requireSession)
	recordsImportCmd.Args = cobra.ExactArgs(3)

	return cmd
}

func runZonesList(ctx *cmdctx.CmdContext) error {
	orgSlug := ctx.Args[0]
	zones, err := ctx.Client.API().GetDNSZones(orgSlug)
	if err != nil {
		return err
	}

	for _, zone := range zones {
		fmt.Println(zone.ID, zone.Domain, zone.CreatedAt)
	}
	return nil
}

func runZonesCreate(ctx *cmdctx.CmdContext) error {
	org, err := ctx.Client.API().FindOrganizationBySlug(ctx.Args[0])
	if err != nil {
		return err
	}
	domain := ctx.Args[1]

	fmt.Printf("Creating zone %s in organization %s\n", domain, org.Slug)

	zone, err := ctx.Client.API().CreateDNSZone(org.ID, domain)
	if err != nil {
		return err
	}

	fmt.Println("Created zone", zone.Domain)

	return nil
}

func runZonesDelete(ctx *cmdctx.CmdContext) error {
	zone, err := ctx.Client.API().FindDNSZone(ctx.Args[0], ctx.Args[1])
	if err != nil {
		return err
	}

	fmt.Printf("Deleting zone %s in organization %s\n", zone.Domain, zone.Organization.Slug)

	err = ctx.Client.API().DeleteDNSZone(zone.ID)
	if err != nil {
		return err
	}

	fmt.Println("Deleted zone", zone.Domain)

	return nil
}

func runRecordsList(ctx *cmdctx.CmdContext) error {
	zone, err := ctx.Client.API().FindDNSZone(ctx.Args[0], ctx.Args[1])
	if err != nil {
		return err
	}

	fmt.Printf("Records for zone %s in organization %s\n", zone.Domain, zone.Organization.Slug)

	records, err := ctx.Client.API().GetDNSRecords(zone.ID)
	if err != nil {
		return err
	}

	for _, record := range records {
		fmt.Println(record.FQDN, record.TTL, record.Type, strings.Join(record.Values, ","))
	}

	return nil
}

func runRecordsExport(ctx *cmdctx.CmdContext) error {
	zone, err := ctx.Client.API().FindDNSZone(ctx.Args[0], ctx.Args[1])
	if err != nil {
		return err
	}

	records, err := ctx.Client.API().ExportDNSRecords(zone.ID)
	if err != nil {
		return err
	}

	fmt.Println(records)

	return nil
}

func runRecordsImport(ctx *cmdctx.CmdContext) error {
	zone, err := ctx.Client.API().FindDNSZone(ctx.Args[0], ctx.Args[1])
	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(ctx.Args[2])
	if err != nil {
		return err
	}

	results, err := ctx.Client.API().ImportDNSRecords(zone.ID, string(data))
	if err != nil {
		return err
	}

	fmt.Println("zonefile imported")

	for _, result := range results {
		fmt.Printf("%s created: %d, updated: %d, deleted: %d, skipped: %d\n", result.Type, result.Created, result.Updated, result.Deleted, result.Skipped)
	}

	return nil
}
