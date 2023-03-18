package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

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
