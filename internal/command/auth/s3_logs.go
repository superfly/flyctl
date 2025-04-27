package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func newS3Logs() *cobra.Command {
	const (
		long  = `Get temporary AWS credentials that can be used for downloading logs from S3`
		short = "Get S3 logs credentials"
		usage = "s3logs"
	)

	cmd := command.New(usage, short, long, runS3Logs,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Org(),
	)

	return cmd
}

func runS3Logs(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	orgSlug := flag.GetOrg(ctx)

	if orgSlug == "" && appName != "" {
		apiClient := flyutil.ClientFromContext(ctx)
		app, err := apiClient.GetAppCompact(ctx, appName)
		if err != nil {
			return fmt.Errorf("failed retrieving app %s: %w", appName, err)
		}
		orgSlug = app.Organization.Slug
	}

	if orgSlug == "" {
		org, err := orgs.OrgFromEnvVarOrFirstArgOrSelect(ctx)
		if err != nil {
			return fmt.Errorf("failed retrieving org: %w", err)
		}
		orgSlug = org.Slug
	}

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		OrgSlug: orgSlug,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	token, err := flapsClient.GetS3LogsToken(ctx, orgSlug)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(io.Out).Encode(token); err != nil {
		return fmt.Errorf("failed decoding response: %w", err)
	}
	return nil
}
