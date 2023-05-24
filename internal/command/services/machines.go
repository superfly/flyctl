package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func ShowMachineServiceInfo(ctx context.Context, app *api.AppInfo) error {
	var (
		io        = iostreams.FromContext(ctx)
		jsonOuput = flag.GetBool(ctx, "json")
	)

	if jsonOuput {
		return fmt.Errorf("outputting to json is not yet supported")
	}

	flapsClient, err := flaps.NewFromAppName(ctx, app.Name)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	machines, err := machine.ListActive(ctx)
	if err != nil {
		return err
	}

	if len(machines) == 0 {
		fmt.Fprintf(io.ErrOut, "No machines found")
		return nil
	}

	serviceList := [][]string{}
	serviceToRegion := map[string][]string{}
	serviceToProcessGroup := map[string][]string{}
	serviceToMachines := map[string]int{}

	services := map[string]struct{}{}

	for _, machine := range machines {
		for _, service := range machine.Config.Services {
			for _, port := range service.Ports {
				protocol := service.Protocol

				handlers := []string{}
				for _, handler := range port.Handlers {
					handlers = append(handlers, strings.ToUpper(handler))
				}

				ports := fmt.Sprintf("%d => %d", *port.Port, service.InternalPort)
				https := cases.Title(language.English, cases.Compact).String(fmt.Sprint(port.ForceHTTPS))
				h := strings.Join(handlers, ",")

				key := getServiceKey(protocol, ports, https, h)

				services[key] = struct{}{}

				serviceToMachines[key]++
				serviceToRegion[key] = append(serviceToRegion[key], machine.Region)
				serviceToProcessGroup[key] = append(serviceToProcessGroup[key], machine.ProcessGroup())
			}
		}
	}

	for service := range services {
		components := strings.Split(service, "-")

		protocol := strings.ToUpper(components[0])
		ports := strings.ToUpper(components[1])
		https := components[2]
		handlers := fmt.Sprintf("[%s]", strings.ToUpper(components[3]))
		processGroup := strings.Join(lo.Uniq(serviceToProcessGroup[service]), ",")
		regions := strings.Join(lo.Uniq(serviceToRegion[service]), ",")
		machineCount := fmt.Sprint(serviceToMachines[service])

		serviceList = append(serviceList, []string{protocol, ports, handlers, https, processGroup, regions, machineCount})
	}

	_ = render.Table(io.Out, "Services", serviceList, "Protocol", "Ports", "Handlers", "Force HTTPS", "Process Group", "Regions", "Machines")

	return nil
}

func getServiceKey(protocol, ports, forcehttps, handlers string) string {
	return fmt.Sprintf("%s-%s-%s-%s", protocol, ports, forcehttps, handlers)
}
