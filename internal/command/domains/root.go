package domains

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Manage domains (deprecated)"
		long  = `Manage domains
Notice: this feature is deprecated and no longer supported.
You can still view existing domains, but registration is no longer possible.`
	)
	cmd := command.New("domains", short, long, nil)
	cmd.Deprecated = "`fly domains` will be removed in a future release"
	cmd.Hidden = true
	cmd.AddCommand(
		newDomainsList(),
		newDomainsShow(),
		newDomainsAdd(),
		newDomainsRegister(),
	)
	cmd.Hidden = true
	return cmd
}

func newDomainsList() *cobra.Command {
	const (
		short = "List domains"
		long  = `List domains for an organization`
	)
	cmd := command.New("list [org]", short, long, runDomainsList,
		command.RequireSession,
	)
	flag.Add(cmd,
		flag.JSONOutput(),
	)
	cmd.Args = cobra.MaximumNArgs(1)
	return cmd
}

func newDomainsShow() *cobra.Command {
	const (
		short = "Show domain"
		long  = `Show information about a domain`
	)
	cmd := command.New("show <domain>", short, long, runDomainsShow,
		command.RequireSession,
	)
	flag.Add(cmd,
		flag.JSONOutput(),
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func newDomainsAdd() *cobra.Command {
	const (
		short = "Add a domain"
		long  = `Add a domain to an organization`
	)
	cmd := command.New("disable", short, long, runDomainsCreate,
		command.RequireSession,
	)
	cmd.Args = cobra.MaximumNArgs(2)
	return cmd
}

func newDomainsRegister() *cobra.Command {
	const (
		short = "Register a domain"
		long  = `Register a new domain in an organization`
	)
	cmd := command.New("register [org] [name]", short, long, runDomainsRegister,
		command.RequireSession,
		command.RequireAppName,
	)
	flag.Add(cmd,
		flag.App(),
		flag.JSONOutput(),
	)
	cmd.Args = cobra.MaximumNArgs(2)
	cmd.Hidden = true
	return cmd
}

func runDomainsList(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	args := flag.Args(ctx)
	var orgSlug string
	if len(args) == 0 {
		org, err := prompt.Org(ctx)
		if err != nil {
			return err
		}
		orgSlug = org.Slug
	} else {
		// TODO: Validity check on org
		orgSlug = args[0]
	}

	domains, err := apiClient.GetDomains(ctx, orgSlug)
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, domains)
		return nil
	}

	table := tablewriter.NewWriter(io.Out)
	table.SetHeader([]string{"Domain", "Registration Status", "DNS Status", "Created"})
	for _, domain := range domains {
		table.Append([]string{domain.Name, *domain.RegistrationStatus, *domain.DnsStatus, format.RelativeTime(domain.CreatedAt)})
	}
	table.Render()

	return nil
}

func runDomainsShow(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()
	name := flag.FirstArg(ctx)

	domain, err := apiClient.GetDomain(ctx, name)
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		render.JSON(io.Out, domain)
		return nil
	}

	fmt.Fprintf(io.Out, "Domain\n")
	fmtstring := "%-20s: %-20s\n"
	fmt.Fprintf(io.Out, fmtstring, "Name", domain.Name)
	fmt.Fprintf(io.Out, fmtstring, "Organization", domain.Organization.Slug)
	fmt.Fprintf(io.Out, fmtstring, "Registration Status", *domain.RegistrationStatus)
	if *domain.RegistrationStatus == "REGISTERED" {
		fmt.Fprintf(io.Out, fmtstring, "Expires At", format.Time(domain.ExpiresAt))

		autorenew := ""
		if *domain.AutoRenew {
			autorenew = "Enabled"
		} else {
			autorenew = "Disabled"
		}

		fmt.Fprintf(io.Out, fmtstring, "Auto Renew", autorenew)
	}

	fmt.Fprintf(io.Out, "\nDNS\n")
	fmt.Fprintf(io.Out, fmtstring, "Status", *domain.DnsStatus)
	if *domain.RegistrationStatus == "UNMANAGED" {
		fmt.Fprintf(io.Out, fmtstring, "Nameservers", strings.Join(*domain.ZoneNameservers, " "))
	}

	return nil
}

func runDomainsCreate(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	apiClient := client.FromContext(ctx).API()

	var org *api.Organization
	var name string
	var err error

	args := flag.Args(ctx)

	if len(args) == 0 {
		org, err = prompt.Org(ctx)
		if err != nil {
			return err
		}

		if err := prompt.String(ctx, &name, "Domain name to add", "", true); err != nil {
			return err
		}

		// TODO: Add some domain validation here
	} else if len(args) == 2 {
		org, err = apiClient.GetOrganizationBySlug(ctx, args[0])
		if err != nil {
			return err
		}
		name = args[1]
	} else {
		return errors.New("specify all arguments (or no arguments to be prompted)")
	}

	fmt.Printf("Creating domain %s in organization %s\n", name, org.Slug)

	domain, err := apiClient.CreateDomain(org.ID, name)
	if err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Created domain", domain.Name)

	return nil
}

func runDomainsRegister(_ context.Context) error {
	return fmt.Errorf("This command is no longer supported.\n")
}
