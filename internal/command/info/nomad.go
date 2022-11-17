package info

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func showNomadInfo(ctx context.Context, info *api.AppInfo) error {
	var (
		jsonOutput = flag.GetBool(ctx, "json")
		io         = iostreams.FromContext(ctx)
	)

	if jsonOutput {
		if err := render.JSON(io.Out, info); err != nil {
			return err
		}
		return nil
	}

	if err := showNomadAppInfo(ctx, info); err != nil {
		return err
	}

	if err := showNomadServiceInfo(ctx, info); err != nil {
		return err
	}

	if err := showNomadIPInfo(ctx, info); err != nil {
		return err
	}

	return nil
}

func showNomadAppInfo(ctx context.Context, info *api.AppInfo) error {
	var (
		io = iostreams.FromContext(ctx)
	)
	rows := [][]string{
		{
			info.Name,
			info.Organization.Slug,
			info.Status,
			fmt.Sprint(info.Version),
			info.PlatformVersion,
			info.Hostname,
		},
	}
	var cols = []string{"Name", "Owner", "Status", "Version", "Platform", "Hostname"}

	if err := render.VerticalTable(io.Out, "App", rows, cols...); err != nil {
		return err
	}

	return nil
}

func showNomadServiceInfo(ctx context.Context, app *api.AppInfo) error {
	var (
		io = iostreams.FromContext(ctx)
	)

	services := [][]string{}
	for _, service := range app.Services {
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
				fmt.Sprintf("%d => %d [%s]", port.Port, service.InternalPort, strings.Join(handlers, ", ")),
			}
			services = append(services, fields)
		}

	}

	_ = render.Table(io.Out, "Services", services, "Protocol", "Ports")

	return nil
}

func showNomadIPInfo(ctx context.Context, info *api.AppInfo) error {
	var (
		io = iostreams.FromContext(ctx)
	)
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
