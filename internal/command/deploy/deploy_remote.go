package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

// newRemoteDeployment is a placeholder for the remote deployment path.
// For now, it returns a not implemented error.
func newRemoteDeployment(ctx context.Context, appConfig *appconfig.Config, img *imgsrc.DeploymentImage) error {
	ctx, span := tracing.GetTracer().Start(ctx, "deploy_to_machines_remote")
	defer span.End()

	// io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	apiClient := flyutil.ClientFromContext(ctx)
	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	uiexClient := uiexutil.ClientFromContext(ctx)
	if uiexClient == nil {
		return fmt.Errorf("uiex client not found in context")
	}

	req := uiex.RemoteDeploymentRequest{
		Organization: appCompact.Organization.Slug,
		Config:       appConfig,
		Image:        img.Tag,
		Strategy:     uiex.RemoteDeploymentStrategyRolling,
		BuildId:      img.BuildID,
	}

	streams := iostreams.FromContext(ctx)
	streams.StartProgressIndicator()

	cmdfmt.PrintBegin(streams.ErrOut, "Waiting for the remote deployer.")

	events, err := uiexClient.CreateDeploy(ctx, appName, req)
	if err != nil {
		return err
	}

	for ev := range events {
		if ev.Type == uiex.DeploymentEventTypeStarted {
			streams.StopProgressIndicator()
			cmdfmt.PrintDone(streams.ErrOut, "Remote deployer ready.")
		}

		fmt.Println("ev", time.Now(), fmt.Sprintf("%+v", ev))
	}

	return nil
}
