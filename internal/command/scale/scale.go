package scale

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"

	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	const (
		short = "Scale app resources"
		long  = `Scale application resources`
	)
	cmd := command.New("scale", short, long, nil)
	cmd.AddCommand(
		newScaleVm(),
		newScaleMemory(),
		newScaleShow(),
		newScaleCount(),
	)
	return cmd
}

func failOnMachinesApp(ctx context.Context) (context.Context, error) {
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)

	app, err := apiClient.GetAppBasic(ctx, appName)
	if err != nil {
		return nil, err
	} else if app.PlatformVersion == appconfig.MachinesPlatform {
		return nil, fmt.Errorf("it looks like your app is running on v2 of our platform, and does not support this legacy command: try running fly machine update instead")
	}

	return ctx, nil
}
