package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func showMachineServiceInfo(ctx context.Context, app *api.AppInfo) error {
	var (
		io        = iostreams.FromContext(ctx)
		client    = client.FromContext(ctx).API()
		jsonOuput = flag.GetBool(ctx, "json")
	)

	if jsonOuput {
		return fmt.Errorf("outputting to json is not yet supported")
	}

	appCompact, err := client.GetAppCompact(ctx, app.Name)
	if err != nil {
		return err
	}

	ctx, err = apps.BuildContext(ctx, appCompact)
	if err != nil {
		return err
	}

	machines, err := machine.ListActive(ctx)
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		fmt.Fprintf(io.ErrOut, "No machines found")
		return nil
	}

	services := [][]string{}

	for _, service := range machines[0].Config.Services {
		for i, port := range service.Ports {
			protocol := service.Protocol
			if i > 0 {
				protocol = ""
			}

			handlers := []string{}
			for _, handler := range port.Handlers {
				handlers = append(handlers, strings.ToUpper(handler))
			}

			fields := []string{
				strings.ToUpper(protocol),
				fmt.Sprintf("%d => %d [%s]", *port.Port, service.InternalPort, strings.Join(handlers, ",")),
				strings.Title(fmt.Sprint(port.ForceHttps)),
			}
			services = append(services, fields)
		}

	}

	_ = render.Table(io.Out, "Services", services, "Protocol", "Ports", "Force HTTPS")

	return nil
}
