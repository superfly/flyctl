package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func newList() *cobra.Command {
	const (
		long  = "List the services that are associated with an app"
		short = "List services"
	)

	cmd := command.New("list", short, long, runList, command.RequireSession, command.RequireAppName)
	cmd.Aliases = []string{"ls"}
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)

	return cmd
}

func runList(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	if flag.GetBool(ctx, "json") {
		return fmt.Errorf("outputting to json is not yet supported")
	}

	machines, err := machine.ListActive(ctx, appName)
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		fmt.Fprintf(io.ErrOut, "No machines found")

		return nil
	}

	type serviceKey struct {
		protocol   string
		ports      string
		forceHTTPS string
		handlers   string
	}

	serviceList := [][]string{}
	serviceToRegion := map[serviceKey][]string{}
	serviceToProcessGroup := map[serviceKey][]string{}
	serviceToMachines := map[serviceKey]int{}

	services := map[serviceKey]struct{}{}

	for _, machine := range machines {
		for _, service := range machine.Config.Services {
			for _, port := range service.Ports {
				protocol := service.Protocol

				handlers := []string{}
				for _, handler := range port.Handlers {
					handlers = append(handlers, strings.ToUpper(handler))
				}

				var externalPorts string
				switch {
				case port.Port != nil:
					externalPorts = fmt.Sprint(*port.Port)
				case port.StartPort != nil || port.EndPort != nil:
					start, end := 0, 65535
					if port.StartPort != nil {
						start = *port.StartPort
					}
					if port.EndPort != nil {
						end = *port.EndPort
					}
					externalPorts = fmt.Sprintf("%d-%d", start, end)
				default:
					return fmt.Errorf("machine %s has a service port without a port or range", machine.ID)
				}
				ports := fmt.Sprintf("%s => %d", externalPorts, service.InternalPort)
				https := cases.Title(language.English, cases.Compact).String(fmt.Sprint(port.ForceHTTPS))
				h := strings.Join(handlers, ",")

				key := serviceKey{protocol: protocol, ports: ports, forceHTTPS: https, handlers: h}

				services[key] = struct{}{}

				serviceToMachines[key]++
				serviceToRegion[key] = append(serviceToRegion[key], machine.Region)
				serviceToProcessGroup[key] = append(serviceToProcessGroup[key], machine.ProcessGroup())
			}
		}
	}

	for service := range services {
		protocol := strings.ToUpper(service.protocol)
		ports := service.ports
		https := service.forceHTTPS
		handlers := fmt.Sprintf("[%s]", strings.ToUpper(service.handlers))
		processGroup := strings.Join(lo.Uniq(serviceToProcessGroup[service]), ",")
		regions := strings.Join(lo.Uniq(serviceToRegion[service]), ",")
		machineCount := fmt.Sprint(serviceToMachines[service])

		serviceList = append(serviceList, []string{protocol, ports, handlers, https, processGroup, regions, machineCount})
	}

	_ = render.Table(io.Out, "Services", serviceList, "Protocol", "Ports", "Handlers", "Force HTTPS", "Process Group", "Regions", "Machines")

	return nil
}
