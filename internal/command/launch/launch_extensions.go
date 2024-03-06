package launch

import (
	"context"

	"github.com/superfly/flyctl/gql"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/secrets"
)

func (state *launchState) launchSentry(ctx context.Context, app_name string) error {
	if state.Plan.Sentry {
		extension, err := extensions_core.ProvisionExtension(ctx, extensions_core.ExtensionParams{
			AppName:  app_name,
			Provider: "sentry",
		})
		if err != nil {
			return err
		}

		if extension.SetsSecrets {
			if err = secrets.DeploySecrets(ctx, gql.ToAppCompact(*extension.App), false, false); err != nil {
				return err
			}
		}
	}

	return nil
}
