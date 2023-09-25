// Package command implements helpers useful for when building cobra commands.
// This source file contains logic common to `fly machine run` and `fly console`
package command

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/slices"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
)

func DetermineImage(ctx context.Context, appName string, imageOrPath string) (img *imgsrc.DeploymentImage, err error) {
	var (
		client = client.FromContext(ctx).API()
		io     = iostreams.FromContext(ctx)
		cfg    = appconfig.ConfigFromContext(ctx)
	)

	daemonType := imgsrc.NewDockerDaemonType(!flag.GetBool(ctx, "build-remote-only"), !flag.GetBool(ctx, "build-local-only"), env.IsCI(), flag.GetBool(ctx, "build-nixpacks"))
	resolver := imgsrc.NewResolver(daemonType, client, appName, io)

	// build if relative or absolute path
	if strings.HasPrefix(imageOrPath, ".") || strings.HasPrefix(imageOrPath, "/") {
		opts := imgsrc.ImageOptions{
			AppName:    appName,
			WorkingDir: path.Join(state.WorkingDirectory(ctx)),
			Publish:    !flag.GetBuildOnly(ctx),
			ImageLabel: flag.GetString(ctx, "image-label"),
			Target:     flag.GetString(ctx, "build-target"),
			NoCache:    flag.GetBool(ctx, "no-build-cache"),
		}

		dockerfilePath := cfg.Dockerfile()

		// dockerfile passed through flags takes precedence over the one set in config
		if flag.GetString(ctx, "dockerfile") != "" {
			dockerfilePath = flag.GetString(ctx, "dockerfile")
		}

		if dockerfilePath != "" {
			dockerfilePath, err := filepath.Abs(dockerfilePath)
			if err != nil {
				return nil, err
			}
			opts.DockerfilePath = dockerfilePath
		}

		extraArgs, err := cmdutil.ParseKVStringsToMap(flag.GetStringArray(ctx, "build-arg"))
		if err != nil {
			return nil, errors.Wrap(err, "invalid build-arg")
		}
		opts.BuildArgs = extraArgs

		img, err = resolver.BuildImage(ctx, io, opts)
		if err != nil {
			return nil, err
		}
		if img == nil {
			return nil, errors.New("could not find an image to deploy")
		}
	} else {
		opts := imgsrc.RefOptions{
			AppName:    appName,
			WorkingDir: state.WorkingDirectory(ctx),
			Publish:    !flag.GetBool(ctx, "build-only"),
			ImageRef:   imageOrPath,
			ImageLabel: flag.GetString(ctx, "image-label"),
		}

		img, err = resolver.ResolveReference(ctx, io, opts)
		if err != nil {
			return nil, err
		}
	}

	if img == nil {
		return nil, errors.New("could not find an image to deploy")
	}

	fmt.Fprintf(io.Out, "Image: %s\n", img.Tag)
	fmt.Fprintf(io.Out, "Image size: %s\n\n", humanize.Bytes(uint64(img.Size)))

	return img, nil
}

func DetermineServices(ctx context.Context, services []api.MachineService) ([]api.MachineService, error) {
	svcKey := func(internalPort int, protocol string) string {
		return fmt.Sprintf("%d/%s", internalPort, protocol)
	}
	servicesRef := lo.Map(services, func(s api.MachineService, _ int) *api.MachineService { return &s })
	servicesMap := lo.KeyBy(servicesRef, func(s *api.MachineService) string {
		return svcKey(s.InternalPort, s.Protocol)
	})

	for _, p := range flag.GetStringSlice(ctx, "port") {
		internalPort, proto, edgePort, edgeStartPort, edgeEndPort, handlers, err := parsePortFlag(p)
		if err != nil {
			return nil, err
		}

		// Look for existing services or append a new one
		svc, ok := servicesMap[svcKey(internalPort, proto)]
		if !ok {
			svc = &api.MachineService{
				InternalPort: internalPort,
				Protocol:     proto,
			}
			servicesRef = append(servicesRef, svc)
			servicesMap[svcKey(internalPort, proto)] = svc
		}

		// A dash handler removes the service: --port 5432/tcp:-
		if slices.Equal(handlers, []string{"-"}) {
			svc.Ports = nil
			continue
		}

		// Look for existing ports and update them
		found := false
		for idx := range svc.Ports {
			svcPort := &svc.Ports[idx]
			if svcPort.Port != nil && edgePort != nil && *(svcPort.Port) == *edgePort {
				found = true
				svcPort.Handlers = handlers
			}
			if svcPort.StartPort != nil && edgeStartPort != nil && *(svcPort.StartPort) == *edgeStartPort {
				found = true
				svcPort.Handlers = handlers
				svcPort.EndPort = edgeEndPort
			}
		}
		// Or append new port definition
		if !found {
			svc.Ports = append(svc.Ports, api.MachinePort{
				Port:      edgePort,
				StartPort: edgeStartPort,
				EndPort:   edgeEndPort,
				Handlers:  handlers,
			})
		}
	}

	// Remove any service without exposed ports
	services = lo.FilterMap(servicesRef, func(s *api.MachineService, _ int) (api.MachineService, bool) {
		if s != nil && len(s.Ports) > 0 {
			return *s, true
		}
		return api.MachineService{}, false
	})

	return services, nil
}

func parsePortFlag(str string) (internalPort int, protocol string, port, startPort, endPort *int, handlers []string, err error) {
	protocol = "tcp"
	splittedPortsProto := strings.Split(str, "/")
	if len(splittedPortsProto) == 2 {
		splittedProtoHandlers := strings.Split(splittedPortsProto[1], ":")
		protocol = splittedProtoHandlers[0]
		handlers = append(handlers, splittedProtoHandlers[1:]...)
	} else if len(splittedPortsProto) > 2 {
		err = errors.New("port must be at most two elements (ports/protocol:handler)")
		return
	}

	port, startPort, endPort, internalPort, err = parsePorts(splittedPortsProto[0])
	if internalPort == 0 {
		switch {
		case port != nil:
			internalPort = *port
		case startPort != nil:
			internalPort = *startPort
		}
	}
	return
}

func parsePorts(input string) (port, start_port, end_port *int, internal_port int, err error) {
	split := strings.Split(input, ":")
	if len(split) == 1 {
		var external_port int
		external_port, err = strconv.Atoi(split[0])
		if err != nil {
			err = errors.Wrap(err, "invalid port")
			return
		}

		port = api.IntPointer(external_port)
	} else if len(split) == 2 {
		internal_port, err = strconv.Atoi(split[1])
		if err != nil {
			err = errors.Wrap(err, "invalid machine (internal) port")
			return
		}

		external_split := strings.Split(split[0], "-")
		if len(external_split) == 1 {
			var external_port int
			external_port, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid external port")
				return
			}

			port = api.IntPointer(external_port)
		} else if len(external_split) == 2 {
			var start int
			start, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid start port for port range")
				return
			}

			start_port = api.IntPointer(start)

			var end int
			end, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid end port for port range")
				return
			}

			end_port = api.IntPointer(end)
		} else {
			err = errors.New("external port must be at most 2 elements (port, or range start-end)")
		}
	} else {
		err = errors.New("port definition must be at most 2 elements (external:internal)")
	}

	return
}
