package tokens

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		short = "List tokens"
		long  = short
		usage = "list"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "scope",
			Shorthand:   "s",
			Description: "either 'app' or 'org'",
			Default:     "",
		},
		flag.Org(),
	)

	cmd.Aliases = []string{"ls"}

	return cmd
}

func runList(ctx context.Context) (err error) {
	apiClient := flyutil.ClientFromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	var rows [][]string

	// Determine scope based on flags passed
	scope := flag.GetString(ctx, "scope")
	appFlag := flag.GetString(ctx, "app")
	configFlag := flag.GetString(ctx, "config")
	orgFlag := flag.GetString(ctx, "org")
	scope, err = determineScope(scope, appFlag, orgFlag, configFlag)
	if err != nil {
		return fmt.Errorf("failed retrieving scope: %w", err)
	}

	// Apply scope to filter list of tokens to display
	switch scope {
	case "app":
		appName := appconfig.NameFromContext(ctx)
		if appName == "" {
			return command.ErrRequireAppName
		}
		// --org passed must match the selected app's org
		if orgFlag != "" {
			// Get app details, so we can identify its organization slug
			app, err := apiClient.GetAppCompact(ctx, appName)
			if err != nil {
				return fmt.Errorf("failed retrieving app %s: %w", appName, err)
			}

			// Get organization details, so we can get its slug
			org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
			if err != nil {
				return fmt.Errorf("failed retrieving org %w", err)
			}

			// Throw an error if app's org slug does not match --org slug
			if app.Organization.Slug != org.Slug {
				return fmt.Errorf("failed to retrieve tokens, selected application \"%s\" does not belong to selected organization \"%s\"", appName, org.Slug)
			}
		}
		tokens, err := apiClient.GetAppLimitedAccessTokens(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed retrieving tokens for app %s: %w", appName, err)
		}

		fmt.Fprintln(out, "Tokens for app \""+appName+"\":")
		for _, token := range tokens {
			rows = append(rows, []string{token.Id, token.Name, token.User.Email, token.ExpiresAt.String(), revokedAtToString(token.RevokedAt)})
		}

	case "org":
		org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
		if err != nil {
			return fmt.Errorf("failed retrieving org %w", err)
		}

		fmt.Fprintln(out, "Tokens for organization \""+org.Slug+"\":")
		for _, token := range org.LimitedAccessTokens.Nodes {
			rows = append(rows, []string{token.Id, token.Name, token.User.Email, token.ExpiresAt.String(), revokedAtToString(token.RevokedAt)})
		}
	}

	_ = render.Table(out, "", rows, "ID", "Name", "Created By", "Expires At", "Revoked At")
	return nil
}

func revokedAtToString(time *time.Time) string {
	if time != nil {
		return time.String()
	}
	return ""
}

func determineScope(scopeStr string, appFlagStr string, orgFlagStr string, configFlagStr string) (scope string, err error) {
	// app scope is given highest priority, as it is more granular than org, identified by --app|--config|--scope=app
	// org scope is only used when specified by --org|--scope=org withought any app scope indicator
	// If scope is neither app nor org, but provided by the user, throw an error; there are only two scopes: app|org
	// the default scope, when all else fails, is app scope
	if appFlagStr != "" || configFlagStr != "" || scopeStr == "app" {
		return "app", nil
	} else if orgFlagStr != "" || scopeStr == "org" {
		return "org", nil
	} else if scopeStr != "" && scopeStr != "app" && scopeStr != "org" {
		return "", fmt.Errorf("Please provide a valid scope: \"app\" or \"org\".")
	} else {
		return "app", nil
	}
}
