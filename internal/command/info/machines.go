package info

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

func showMachineInfo(ctx context.Context, appName string) error {
	var (
		client    = client.FromContext(ctx).API()
		jsonOuput = flag.GetBool(ctx, "json")
	)

	if jsonOuput {
		return fmt.Errorf("outputting to json is not yet supported")
	}

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}

	ctx, err = apps.BuildContext(ctx, app)
	if err != nil {
		return err
	}

	if err := showMachineAppInfo(ctx, app); err != nil {
		return err
	}

	if err := showMachineServiceInfo(ctx, app); err != nil {
		return err
	}

	if err := showMachineIPInfo(ctx, appName); err != nil {
		return err
	}

	return nil
}

func showMachineAppInfo(ctx context.Context, app *api.AppCompact) error {
	var (
		io = iostreams.FromContext(ctx)
	)
	rows := [][]string{
		{
			app.Name,
			app.Organization.Slug,
			app.PlatformVersion,
			app.Hostname,
		},
	}
	var cols = []string{"Name", "Owner", "Platform", "Hostname"}

	if err := render.VerticalTable(io.Out, "App", rows, cols...); err != nil {
		return err
	}

	return nil
}

func showMachineServiceInfo(ctx context.Context, app *api.AppCompact) error {
	var (
		io = iostreams.FromContext(ctx)
	)

	machines, err := machine.ListActive(ctx)
	if err != nil {
		return err
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
				fmt.Sprintf("%d => %d [%s]", port.Port, service.InternalPort, strings.Join(handlers, ",")),
				strings.Title(fmt.Sprint(port.ForceHttps)),
			}
			services = append(services, fields)
		}

	}

	_ = render.Table(io.Out, "Services", services, "Protocol", "Ports", "Force HTTPS")

	return nil
}

func showMachineIPInfo(ctx context.Context, appName string) error {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	info, err := client.GetAppInfo(ctx, appName)
	if err != nil {
		return err
	}

	ips := [][]string{}

	for _, ip := range info.IPAddresses.Nodes {
		fields := []string{
			ip.Type,
			ip.Address,
			ip.Region,
			ip.CreatedAt.String(),
		}
		ips = append(ips, fields)
	}

	_ = render.Table(io.Out, "IP Addresses", ips, "Type", "Address", "Region", "Createde at")

	return nil
}
