package tokens

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"

	fly "github.com/superfly/fly-go"
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
			Default:     "app",
		},
		flag.Org(),
	)

	return cmd
}

func runList(ctx context.Context) (err error) {
	apiClient := flyutil.ClientFromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	var rows [][]string

	// Get flags
	
	// Organization Name
	orgFlag := flag.GetString(ctx, "org")
	var org *fly.Organization
	var scope string
	if orgFlag!=""{
		// Get the org name if flag indicated
		org, err = orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
		if err != nil {
			return fmt.Errorf("failed retrieving org %w", err)
		}
		scope = "org"
	}
	fmt.Println( "ORG IS: ")
	fmt.Println( org)

	// App Name
	appFlag := flag.GetString(ctx, "app")
	var appName string
	if orgFlag == "" || appFlag!="" {
		// BY default( orgflag not passed ), OR when --a passed
		appName = appconfig.NameFromContext(ctx)
		if appName == ""{
			return command.ErrRequireAppName
		}

		// Check if app belongs to org if it exists
		if org != nil {
			app, err := apiClient.GetAppCompact(ctx, appName)
			if err != nil {
				return fmt.Errorf("failed retrieving app %s: %w", appName, err)
			}
			fmt.Println("checking app's org: ")
			fmt.Println(app)

			
			if app.Organization.Name != org.Name {
				return fmt.Errorf("APp does not belong to org!")
			}else{
				scope = "org"
			}

		}else{
			scope = "app"
		}
	}

	
	switch scope {
	case "app":
		appName := appconfig.NameFromContext(ctx)
		if appName == "" {
			return command.ErrRequireAppName
		}

		tokens, err := apiClient.GetAppLimitedAccessTokens(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed retrieving tokens for app %s: %w", appName, err)
		}

		for _, token := range tokens {
			rows = append(rows, []string{token.Id, token.Name, token.User.Email, token.ExpiresAt.String()})
		}

	case "org":
		org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
		if err != nil {
			return fmt.Errorf("failed retrieving org %w", err)
		}

		for _, token := range org.LimitedAccessTokens.Nodes {
			rows = append(rows, []string{token.Id, token.Name, token.User.Email, token.ExpiresAt.String()})
		}
	}
	_ = render.Table(out, "", rows, "ID", "Name", "Created By", "Expires At")
	return nil
}
