package supabase

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func create() (cmd *cobra.Command) {

	const (
		short = "Provision a Supabase Postgres database"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runCreate, command.RequireSession, command.LoadAppNameIfPresent)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
		flag.Region(),
		extensions_core.SharedFlags,
		flag.String{
			Name:        "name",
			Shorthand:   "n",
			Description: "The name of your database",
		},
	)
	return cmd
}

func CaptureFreeLimitError(ctx context.Context, provisioningError error, params *extensions_core.ExtensionParams) error {
	io := iostreams.FromContext(ctx)
	if strings.Contains(provisioningError.Error(), "limited to one") {
		fmt.Fprintf(io.Out, "You're limited to one free database across all of your Supabase organizations. To provision another db, you can upgrade the selected Supabase organization to the $25/mo Pro Plan. Get more details at https://supabase.com/docs/guides/platform/org-based-billing.\n\n")
		confirm, err := prompt.Confirm(ctx, "Would you like to upgrade your Supabase organization now ($25/mo, prorated) and launch a database?")

		if err != nil {
			return err
		}

		if confirm {
			params.OrganizationPlanID = "pro"
			_, err := extensions_core.ProvisionExtension(ctx, *params)

			if err != nil {
				return err
			}
		}
	}

	return provisioningError
}

func runCreate(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)

	params := extensions_core.ExtensionParams{}

	if appName != "" {
		params.AppName = appName
	} else {
		org, err := orgs.OrgFromFlagOrSelect(ctx)

		if err != nil {
			return err
		}

		params.Organization = org
	}

	params.Provider = "supabase"
	params.ErrorCaptureCallback = CaptureFreeLimitError
	extension, err := extensions_core.ProvisionExtension(ctx, params)

	if err != nil {
		return err
	}

	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, gql.ToAppCompact(*extension.App), false, false)
	}

	return
}
