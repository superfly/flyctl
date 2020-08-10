package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
)

func newDNSCommand() *Command {
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
			Use:     zonesStrings.Usage,
			Short:   zonesStrings.Short,
			Long:    zonesStrings.Long,
			Aliases: []string{"z"},
		},
	}
	cmd.AddCommand(zonesCmd)

	zonesListStrings := docstrings.Get("dns.zones.list")
	zonesListCmd := BuildCommandKS(zonesCmd, runZonesList, zonesListStrings, os.Stdout, requireSession)
	zonesListCmd.Args = cobra.MaximumNArgs(1)

	zonesCreateStrings := docstrings.Get("dns.zones.create")
	zonesCreateCmd := BuildCommandKS(zonesCmd, runZonesCreate, zonesCreateStrings, os.Stdout, requireSession)
	zonesCreateCmd.Args = cobra.MaximumNArgs(2)

	zonesDeleteStrings := docstrings.Get("dns.zones.delete")
	zonesDeleteCmd := BuildCommandKS(zonesCmd, runZonesDelete, zonesDeleteStrings, os.Stdout, requireSession)
	zonesDeleteCmd.Args = cobra.MaximumNArgs(2)

	recordsStrings := docstrings.Get("dns.records")
	recordsCmd := &Command{
		Command: &cobra.Command{
			Use:     recordsStrings.Usage,
			Short:   recordsStrings.Short,
			Long:    recordsStrings.Long,
			Aliases: []string{"r"},
		},
	}
	cmd.AddCommand(recordsCmd)

	recordsListStrings := docstrings.Get("dns.records.list")
	recordsListCmd := BuildCommandKS(recordsCmd, runRecordsList, recordsListStrings, os.Stdout, requireSession)
	recordsListCmd.Args = cobra.MaximumNArgs(2)

	recordsExportStrings := docstrings.Get("dns.records.export")
	recordsExportCmd := BuildCommandKS(recordsCmd, runRecordsExport, recordsExportStrings, os.Stdout, requireSession)
	recordsExportCmd.Args = cobra.MaximumNArgs(2)

	recordsImportStrings := docstrings.Get("dns.records.import")
	recordsImportCmd := BuildCommandKS(recordsCmd, runRecordsImport, recordsImportStrings, os.Stdout, requireSession)
	recordsImportCmd.Args = cobra.MaximumNArgs(3)

	return cmd
}

func runZonesList(ctx *cmdctx.CmdContext) error {
	var orgSlug string
	if len(ctx.Args) == 0 {
		org, err := selectOrganization(ctx.Client.API(), "")
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	} else {
		orgSlug = ctx.Args[0]
	}
	zones, err := ctx.Client.API().GetDNSZones(orgSlug)
	if err != nil {
		return err
	}

	zonetable := tablewriter.NewWriter(ctx.Out)

	zonetable.SetHeader([]string{"Domain", "Created"})

	for _, zone := range zones {
		zonetable.Append([]string{zone.Domain, fmt.Sprintf("%s (%s)", humanize.Time(zone.CreatedAt), zone.CreatedAt.Format(time.UnixDate))})
	}

	zonetable.Render()

	return nil
}

func runZonesCreate(ctx *cmdctx.CmdContext) error {
	var org *api.Organization
	var domain string
	var err error

	if len(ctx.Args) == 0 {
		org, err = selectOrganization(ctx.Client.API(), "")
		if err != nil {
			return err
		}

		prompt := &survey.Input{Message: "Domain name to create"}
		survey.AskOne(prompt, &domain)
		// TODO: Add some domain validation here
	} else if len(ctx.Args) == 2 {
		org, err = ctx.Client.API().FindOrganizationBySlug(ctx.Args[0])
		if err != nil {
			return err
		}
		domain = ctx.Args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	fmt.Printf("Creating zone %s in organization %s\n", domain, org.Slug)

	zone, err := ctx.Client.API().CreateDNSZone(org.ID, domain)
	if err != nil {
		return err
	}

	fmt.Println("Created zone", zone.Domain)

	return nil
}

func runZonesDelete(ctx *cmdctx.CmdContext) error {
	var org *api.Organization
	var domain string
	var err error

	if len(ctx.Args) == 0 {
		org, err = selectOrganization(ctx.Client.API(), "")
		if err != nil {
			return err
		}

		prompt := &survey.Input{Message: "Domain name to delete"}
		survey.AskOne(prompt, &domain)
	} else if len(ctx.Args) == 2 {
		org, err = ctx.Client.API().FindOrganizationBySlug(ctx.Args[0])
		if err != nil {
			return err
		}
		domain = ctx.Args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	zone, err := ctx.Client.API().FindDNSZone(org.Slug, domain)

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
	var org *api.Organization
	var zoneslug string
	var err error

	if len(ctx.Args) == 0 {
		org, err = selectOrganization(ctx.Client.API(), "")
		if err != nil {
			return err
		}

		zoneslug, err = selectZone(ctx.Client.API(), org.Slug, "")

	} else if len(ctx.Args) == 2 {
		org, err = ctx.Client.API().FindOrganizationBySlug(ctx.Args[0])
		if err != nil {
			return err
		}
		zoneslug = ctx.Args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	zone, err := ctx.Client.API().FindDNSZone(org.Slug, zoneslug)

	if err != nil {
		return err
	}

	fmt.Printf("Records for zone %s in organization %s\n", zone.Domain, zone.Organization.Slug)

	records, err := ctx.Client.API().GetDNSRecords(zone.ID)
	if err != nil {
		return err
	}

	if ctx.OutputJSON() {
		ctx.WriteJSON(records)
		return nil
	}

	recordtable := tablewriter.NewWriter(ctx.Out)
	recordtable.SetHeader([]string{"FQDN", "TTL", "Type", "Values"})

	for _, record := range records {
		recordtable.Append([]string{record.FQDN, strconv.Itoa(record.TTL), record.Type, strings.Join(record.Values, ",")})
	}

	recordtable.Render()

	return nil
}

func runRecordsExport(ctx *cmdctx.CmdContext) error {
	var org *api.Organization
	var zoneslug string
	var err error

	if len(ctx.Args) == 0 {
		org, err = selectOrganization(ctx.Client.API(), "")
		if err != nil {
			return err
		}

		zoneslug, err = selectZone(ctx.Client.API(), org.Slug, "")

	} else if len(ctx.Args) == 2 {
		org, err = ctx.Client.API().FindOrganizationBySlug(ctx.Args[0])
		if err != nil {
			return err
		}
		zoneslug = ctx.Args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	zone, err := ctx.Client.API().FindDNSZone(org.Slug, zoneslug)
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
	var org *api.Organization
	var zoneslug string
	var filename string

	var err error

	if len(ctx.Args) == 0 {
		org, err = selectOrganization(ctx.Client.API(), "")
		if err != nil {
			return err
		}

		zoneslug, err = selectZone(ctx.Client.API(), org.Slug, "")

		validateFile := func(val interface{}) error {
			_, err := os.Stat(val.(string))
			if os.IsNotExist(err) {
				return fmt.Errorf("File %s does not exist", val.(string))
			}
			return nil
		}

		err := survey.AskOne(&survey.Input{Message: "Import filename:"}, &filename, survey.WithValidator(validateFile))

		if err != nil {
			return err
		}

	} else if len(ctx.Args) == 3 {
		org, err = ctx.Client.API().FindOrganizationBySlug(ctx.Args[0])
		if err != nil {
			return err
		}
		zoneslug = ctx.Args[1]
		filename = ctx.Args[2]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	zone, err := ctx.Client.API().FindDNSZone(org.Slug, zoneslug)
	if err != nil {
		return err
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	results, err := ctx.Client.API().ImportDNSRecords(zone.ID, string(data))
	if err != nil {
		return err
	}

	fmt.Println("Zonefile import report")

	for _, result := range results {
		fmt.Printf("%s created: %d, updated: %d, deleted: %d, skipped: %d\n", result.Type, result.Created, result.Updated, result.Deleted, result.Skipped)
	}

	return nil
}
