package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func newOIDC() *cobra.Command {
	const (
		long = `Get OIDC token with specified audience and optional AWS configuration.
This allows fetching an OIDC token suitable for authentication against
systems configured to trust the Fly.io OIDC provider.`
		short = "Get OIDC token"
		usage = "oidc"
	)

	cmd := command.New(usage, short, long, runOIDC,
		command.RequireSession,
		command.RequireAppName,
	)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "aud",
			Description: "Audience for the OIDC token",
		},
		flag.Bool{
			Name:        "aws",
			Description: "Use AWS OIDC configuration",
		},
	)

	return cmd
}

func runOIDC(ctx context.Context) error {
	useAWS := flag.GetBool(ctx, "aws")
	audience := flag.GetString(ctx, "aud")
	io := iostreams.FromContext(ctx)

	if audience == "" {
		if useAWS {
			audience = "sts.amazonaws.com"
		} else {
			return fmt.Errorf("audience (--aud) is required")
		}
	}

	appName := appconfig.NameFromContext(ctx)

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
	if err != nil {
		return err
	}
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	token, err := flapsClient.GetOIDCToken(ctx, audience, useAWS)
	if err != nil {
		return err
	}
	fmt.Fprintln(io.Out, token)

	return nil
}
